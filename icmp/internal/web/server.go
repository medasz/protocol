package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"protocol/icmp/frontend"
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
	hub *Hub
}

func NewServer(hub *Hub) *Server {
	return &Server{hub: hub}
}

func (s *Server) Start(addr string) error {
	http.HandleFunc("/api/agents", s.handleAgents)
	http.HandleFunc("/ws/terminal", s.handleTerminal)

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
