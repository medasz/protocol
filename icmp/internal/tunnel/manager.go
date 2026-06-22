package tunnel

import (
	"context"
	"net"
	"sync"

	"protocol/icmp/internal/protocol"
)

// SendFunc is a callback provided by the transport layer to send ICMP payload data.
type SendFunc func(ctx context.Context, payload []byte) error

// TunnelManager multiplexes ICMP packets to different ICMPConn sessions.
type TunnelManager struct {
	mu       sync.RWMutex
	sessions map[uint32]*ICMPConn
	sender   SendFunc
}

// NewTunnelManager creates a new TunnelManager.
func NewTunnelManager(sender SendFunc) *TunnelManager {
	return &TunnelManager{
		sessions: make(map[uint32]*ICMPConn),
		sender:   sender,
	}
}

// HandlePacket processes an incoming tunnel packet and routes it to the correct session.
func (m *TunnelManager) HandlePacket(b []byte) {
	if len(b) < protocol.TunnelHeaderSize {
		return
	}
	header, err := protocol.UnmarshalTunnelHeader(b)
	if err != nil {
		return
	}

	m.mu.RLock()
	conn, exists := m.sessions[header.SessionID]
	m.mu.RUnlock()

	if !exists {
		// If it's a SYN, maybe we should create a new connection (Server mode).
		// For now, let's keep it simple and assume connections are created manually or via SYN.
		if header.Type == protocol.TunnelTypeSYN {
			conn = newICMPConn(header.SessionID, m.sender)
			m.mu.Lock()
			m.sessions[header.SessionID] = conn
			m.mu.Unlock()
			// Handle SYN by responding with ACK or similar logic
		} else {
			return // Ignore packets for unknown sessions unless it's a SYN
		}
	}

	// Dispatch packet payload to the connection
	if header.Type == protocol.TunnelTypeFIN {
		conn.Close()
		m.mu.Lock()
		delete(m.sessions, header.SessionID)
		m.mu.Unlock()
		return
	}
	
	if header.Length > 0 && int(protocol.TunnelHeaderSize+header.Length) <= len(b) {
		payload := b[protocol.TunnelHeaderSize : protocol.TunnelHeaderSize+header.Length]
		conn.handleIncomingPacket(header, payload)
	} else if header.Type == protocol.TunnelTypeACK {
		conn.handleIncomingPacket(header, nil)
	}
}

// Dial creates a new outbound ICMPConn.
func (m *TunnelManager) Dial(sessionID uint32) net.Conn {
	m.mu.Lock()
	defer m.mu.Unlock()
	conn := newICMPConn(sessionID, m.sender)
	m.sessions[sessionID] = conn
	return conn
}
