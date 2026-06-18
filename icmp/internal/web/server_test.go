package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestHubTracksAgentsAndRoutesCommands(t *testing.T) {
	hub := NewHub()
	hub.TouchAgent("10.0.0.2", "00:11:22:33:44:55")
	hub.TouchAgent("10.0.0.3", "")

	if agents := hub.Agents(); len(agents) != 2 {
		t.Fatalf("agents length = %d, want 2", len(agents))
	}

	hub.EnqueueCommand("10.0.0.2", []byte("whoami"))
	hub.EnqueueCommand("10.0.0.3", []byte("hostname"))

	cmd, err := hub.NextCommand(context.Background(), "10.0.0.2")
	if err != nil {
		t.Fatalf("NextCommand(10.0.0.2) error = %v", err)
	}
	if got, want := string(cmd), "whoami"; got != want {
		t.Fatalf("command for 10.0.0.2 = %q, want %q", got, want)
	}
	cmd, err = hub.NextCommand(context.Background(), "10.0.0.3")
	if err != nil {
		t.Fatalf("NextCommand(10.0.0.3) error = %v", err)
	}
	if got, want := string(cmd), "hostname"; got != want {
		t.Fatalf("command for 10.0.0.3 = %q, want %q", got, want)
	}
}

func TestHubBroadcastsOutputToAgentSubscribers(t *testing.T) {
	hub := NewHub()
	hub.TouchAgent("10.0.0.2", "")
	hub.TouchAgent("10.0.0.3", "")
	sub2, unsubscribe2, err := hub.Subscribe("10.0.0.2")
	if err != nil {
		t.Fatalf("Subscribe(10.0.0.2) error = %v", err)
	}
	defer unsubscribe2()
	sub3, unsubscribe3, err := hub.Subscribe("10.0.0.3")
	if err != nil {
		t.Fatalf("Subscribe(10.0.0.3) error = %v", err)
	}
	defer unsubscribe3()

	if err := hub.WriteResult("10.0.0.2", []byte("alice")); err != nil {
		t.Fatalf("WriteResult() error = %v", err)
	}

	select {
	case got := <-sub2:
		if string(got) != "alice" {
			t.Fatalf("subscriber received %q, want alice", string(got))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for 10.0.0.2 output")
	}
	select {
	case got := <-sub3:
		t.Fatalf("10.0.0.3 subscriber received unexpected output %q", string(got))
	default:
	}
}

func TestHandleAgentsReturnsHubAgents(t *testing.T) {
	hub := NewHub()
	hub.TouchAgent("10.0.0.2", "00:11:22:33:44:55")
	server := NewServer(hub)

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rec := httptest.NewRecorder()
	server.handleAgents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var agents []Agent
	if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if len(agents) != 1 || agents[0].IP != "10.0.0.2" || agents[0].MAC != "00:11:22:33:44:55" {
		t.Fatalf("agents = %+v, want 10.0.0.2 with mac", agents)
	}
}

func TestHandleTerminalRejectsMissingOrUnknownAgent(t *testing.T) {
	server := NewServer(NewHub())
	ts := httptest.NewServer(http.HandlerFunc(server.handleTerminal))
	defer ts.Close()

	for _, path := range []string{"/ws/terminal", "/ws/terminal?ip=10.0.0.2"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s error = %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("GET %s status = %d, want non-200", path, resp.StatusCode)
		}
	}
}

func TestHandleTerminalRoutesWebSocketByAgent(t *testing.T) {
	hub := NewHub()
	hub.TouchAgent("10.0.0.2", "")
	server := NewServer(hub)
	ts := httptest.NewServer(http.HandlerFunc(server.handleTerminal))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/terminal?ip=10.0.0.2"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte("whoami")); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	cmd := waitForCommand(t, hub, "10.0.0.2")
	if got, want := string(cmd), "whoami"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}

	if err := hub.WriteResult("10.0.0.2", []byte("alice")); err != nil {
		t.Fatalf("WriteResult() error = %v", err)
	}
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if got, want := string(msg), "alice"; got != want {
		t.Fatalf("websocket output = %q, want %q", got, want)
	}
}

func waitForCommand(t *testing.T, hub *Hub, agentIP string) []byte {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		cmd, err := hub.NextCommand(context.Background(), agentIP)
		if err != nil {
			t.Fatalf("NextCommand() error = %v", err)
		}
		if len(cmd) > 0 {
			return cmd
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timeout waiting for command")
	return nil
}
