package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"io"
	"log"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"protocol/icmp/frontend"
	"protocol/icmp/internal/socks"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins for dev
}

var ErrAgentNotFound = errors.New("agent not found")

type Agent struct {
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	LastSeen int64  `json:"lastSeen"`
	Online   bool   `json:"online"`
}

type agentSession struct {
	agent       Agent
	commands    chan []byte
	subscribers map[chan []byte]struct{}
}

type Hub struct {
	mu       sync.RWMutex
	sessions map[string]*agentSession
	now      func() time.Time
}

func NewHub() *Hub {
	return &Hub{
		sessions: make(map[string]*agentSession),
		now:      time.Now,
	}
}

func (h *Hub) TouchAgent(agentIP string, mac string) {
	if agentIP == "" || agentIP == "<nil>" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	session := h.ensureSessionLocked(agentIP)
	session.agent.IP = agentIP
	if mac != "" && mac != "<nil>" {
		session.agent.MAC = mac
	}
	session.agent.LastSeen = h.now().UnixMilli()
	session.agent.Online = true
}

func (h *Hub) Agents() []Agent {
	h.mu.RLock()
	defer h.mu.RUnlock()
	agents := make([]Agent, 0, len(h.sessions))
	for _, session := range h.sessions {
		agents = append(agents, session.agent)
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].IP < agents[j].IP
	})
	return agents
}

func (h *Hub) EnqueueCommand(agentIP string, data []byte) error {
	h.mu.RLock()
	session := h.sessions[agentIP]
	h.mu.RUnlock()
	if session == nil {
		return ErrAgentNotFound
	}
	select {
	case session.commands <- bytes.Clone(data):
	default:
		return errors.New("command queue full")
	}
	return nil
}

func (h *Hub) NextCommand(ctx context.Context, agentIP string) ([]byte, error) {
	h.mu.RLock()
	session := h.sessions[agentIP]
	h.mu.RUnlock()
	if session == nil {
		return nil, nil
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case cmd := <-session.commands:
		return bytes.Clone(cmd), nil
	default:
		return nil, nil
	}
}

func (h *Hub) WriteResult(agentIP string, result []byte) error {
	h.mu.RLock()
	session := h.sessions[agentIP]
	if session == nil {
		h.mu.RUnlock()
		return ErrAgentNotFound
	}
	subscribers := make([]chan []byte, 0, len(session.subscribers))
	for sub := range session.subscribers {
		subscribers = append(subscribers, sub)
	}
	h.mu.RUnlock()

	for _, sub := range subscribers {
		select {
		case sub <- bytes.Clone(result):
		default:
		}
	}
	return nil
}

func (h *Hub) Subscribe(agentIP string) (<-chan []byte, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	session := h.sessions[agentIP]
	if session == nil {
		return nil, nil, ErrAgentNotFound
	}
	ch := make(chan []byte, 32)
	session.subscribers[ch] = struct{}{}
	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if current := h.sessions[agentIP]; current != nil {
			delete(current.subscribers, ch)
		}
		close(ch)
	}
	return ch, unsubscribe, nil
}

func (h *Hub) ensureSessionLocked(agentIP string) *agentSession {
	session := h.sessions[agentIP]
	if session == nil {
		session = &agentSession{
			agent:       Agent{IP: agentIP},
			commands:    make(chan []byte, 100),
			subscribers: make(map[chan []byte]struct{}),
		}
		h.sessions[agentIP] = session
	}
	return session
}

type Server struct {
	hub         *Hub
	dialer      func(target string) (net.Conn, error)
	dialPty     func() (net.Conn, error)
	servicesMu  sync.Mutex
	activeSocks map[string]net.Listener
	activeFwd   map[string]net.Listener
}

func NewServer(hub *Hub, dialer func(target string) (net.Conn, error), dialPty func() (net.Conn, error)) *Server {
	return &Server{
		hub:         hub,
		dialer:      dialer,
		dialPty:     dialPty,
		activeSocks: make(map[string]net.Listener),
		activeFwd:   make(map[string]net.Listener),
	}
}

func (s *Server) Start(addr string) error {
	http.HandleFunc("/api/agents", s.handleAgents)
	http.HandleFunc("/api/services", s.handleServices)
	http.HandleFunc("/api/services/socks", s.handleSocks)
	http.HandleFunc("/api/services/fwd", s.handleFwd)
	http.HandleFunc("/ws/terminal", s.handleTerminal)
	http.HandleFunc("/ws/pty", s.handlePty)

	dist, err := fs.Sub(frontend.DistFS, "dist")
	if err != nil {
		return err
	}
	http.Handle("/", http.FileServer(http.FS(dist)))

	log.Printf("Web server listening on http://%s", addr)
	return http.ListenAndServe(addr, nil)
}

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.hub.Agents())
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	s.servicesMu.Lock()
	defer s.servicesMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	
	type ServiceInfo struct {
		Type   string `json:"type"`
		Port   string `json:"port"`
		Target string `json:"target,omitempty"`
	}
	var res []ServiceInfo
	for p := range s.activeSocks {
		res = append(res, ServiceInfo{Type: "socks5", Port: p})
	}
	for p := range s.activeFwd {
		// Just returning the port for simplicity, could map to target
		res = append(res, ServiceInfo{Type: "fwd", Port: p})
	}
	json.NewEncoder(w).Encode(res)
}

func (s *Server) handleSocks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, DELETE")
		w.WriteHeader(http.StatusOK)
		return
	}
	
	var req struct{ Port string `json:"port"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	s.servicesMu.Lock()
	defer s.servicesMu.Unlock()
	
	if r.Method == http.MethodPost {
		if _, ok := s.activeSocks[req.Port]; ok {
			http.Error(w, "already running", http.StatusConflict)
			return
		}
		l, err := net.Listen("tcp", ":"+req.Port)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		srv := socks.NewServer(":"+req.Port, s.dialer)
		go srv.Start(l)
		s.activeSocks[req.Port] = l
		w.WriteHeader(http.StatusOK)
		
	} else if r.Method == http.MethodDelete {
		if l, ok := s.activeSocks[req.Port]; ok {
			l.Close()
			delete(s.activeSocks, req.Port)
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (s *Server) handleFwd(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "POST, DELETE")
		w.WriteHeader(http.StatusOK)
		return
	}
	
	var req struct {
		LocalPort string `json:"localPort"`
		Target    string `json:"target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	s.servicesMu.Lock()
	defer s.servicesMu.Unlock()
	
	if r.Method == http.MethodPost {
		if _, ok := s.activeFwd[req.LocalPort]; ok {
			http.Error(w, "already running", http.StatusConflict)
			return
		}
		l, err := net.Listen("tcp", ":"+req.LocalPort)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.activeFwd[req.LocalPort] = l
		go func(listener net.Listener, target string) {
			for {
				c, err := listener.Accept()
				if err != nil {
					return
				}
				go func(conn net.Conn) {
					defer conn.Close()
					targetConn, err := s.dialer(target)
					if err != nil {
						return
					}
					defer targetConn.Close()
					go func() { _, _ = io.Copy(targetConn, conn) }()
					_, _ = io.Copy(conn, targetConn)
				}(c)
			}
		}(l, req.Target)
		w.WriteHeader(http.StatusOK)
		
	} else if r.Method == http.MethodDelete {
		if l, ok := s.activeFwd[req.LocalPort]; ok {
			l.Close()
			delete(s.activeFwd, req.LocalPort)
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	agentIP := r.URL.Query().Get("ip")
	if agentIP == "" {
		http.Error(w, "missing agent ip", http.StatusBadRequest)
		return
	}
	outputs, unsubscribe, err := s.hub.Subscribe(agentIP)
	if err != nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	defer unsubscribe()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WS Upgrade error:", err)
		return
	}
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("error: %v", err)
				}
				return
			}
			if err := s.hub.EnqueueCommand(agentIP, message); err != nil {
				log.Printf("enqueue command for %s: %v", agentIP, err)
				return
			}
		}
	}()

	for {
		select {
		case <-done:
			return
		case output := <-outputs:
			if err := conn.WriteMessage(websocket.TextMessage, output); err != nil {
				return
			}
		}
	}
}

func (s *Server) handlePty(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WS Upgrade error:", err)
		return
	}
	defer conn.Close()

	if s.dialPty == nil {
		return
	}
	ptyConn, err := s.dialPty()
	if err != nil {
		return
	}
	defer ptyConn.Close()

	// ws -> pty
	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if _, err := ptyConn.Write(message); err != nil {
				return
			}
		}
	}()

	// pty -> ws
	buf := make([]byte, 4096)
	for {
		n, err := ptyConn.Read(buf)
		if err != nil {
			return
		}
		if n > 0 {
			if err := conn.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
				return
			}
		}
	}
}
