package web

import (
	"context"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"protocol/icmp/frontend"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins for dev
}

type WsBridge struct {
	conn     *websocket.Conn
	cmdChan  chan []byte
}

func NewWsBridge() *WsBridge {
	return &WsBridge{
		cmdChan: make(chan []byte, 100),
	}
}

// NextCommand implements app.CommandSource
func (b *WsBridge) NextCommand(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case cmd := <-b.cmdChan:
		return cmd, nil
	default:
		// Do not block if there's no command ready!
		return nil, nil
	}
}

// WriteResult implements app.ResultSink
func (b *WsBridge) WriteResult(result []byte) error {
	if b.conn != nil {
		return b.conn.WriteMessage(websocket.TextMessage, result)
	}
	return nil
}

type Server struct {
	bridge  *WsBridge
	agentIp string
}

func NewServer(bridge *WsBridge, agentIp string) *Server {
	return &Server{
		bridge:  bridge,
		agentIp: agentIp,
	}
}

func (s *Server) Start(addr string) error {
	http.HandleFunc("/api/agents", s.handleAgents)
	http.HandleFunc("/ws/terminal", s.handleTerminal)
	
	// Serve static files from embedded frontend/dist
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
	agentIp := s.agentIp
	if agentIp == "" {
		agentIp = "127.0.0.1" // fallback
	}
	json.NewEncoder(w).Encode([]map[string]interface{}{
		{"ip": agentIp, "mac": "Unknown", "online": true},
	})
}

func (s *Server) handleTerminal(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WS Upgrade error:", err)
		return
	}
	s.bridge.conn = conn

	defer func() {
		s.bridge.conn = nil
		conn.Close()
	}()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		s.bridge.cmdChan <- message
	}
}
