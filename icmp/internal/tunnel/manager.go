package tunnel

import (
	"context"
	"net"
	"sync"
	"time"

	"protocol/icmp/internal/protocol"
)

// SendFunc is a callback provided by the transport layer to send ICMP payload data.
type SendFunc func(ctx context.Context, payload []byte) error

// TunnelManager multiplexes ICMP packets to different ICMPConn sessions.
type TunnelManager struct {
	mu       sync.RWMutex
	sessions map[uint32]*ICMPConn
	outbound chan []byte
	sender   SendFunc

	// OnSYN is called when a new SYN packet is received.
	// The callback receives the new connection and the SYN payload.
	OnSYN func(conn net.Conn, payload []byte)
}

// NewTunnelManager creates a new TunnelManager.
func NewTunnelManager() *TunnelManager {
	m := &TunnelManager{
		sessions: make(map[uint32]*ICMPConn),
		outbound: make(chan []byte, 1000),
	}
	m.sender = func(ctx context.Context, payload []byte) error {
		select {
		case m.outbound <- append([]byte(nil), payload...):
		default:
			// Queue full, drop packet (ARQ will retry)
		}
		return nil
	}
	return m
}

// TryDequeue returns an outbound packet if available.
func (m *TunnelManager) TryDequeue() []byte {
	select {
	case p := <-m.outbound:
		m.updateSentAt(p)
		return p
	default:
		return nil
	}
}

// updateSentAt parses the tunnel packet header and updates the packet's sentAt
// timestamp to now, indicating it has been sent on the wire and ARQ timeout should begin.
func (m *TunnelManager) updateSentAt(b []byte) {
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
	if exists {
		conn.unackedMu.Lock()
		if p, ok := conn.unacked[header.Seq]; ok {
			p.sentAt = time.Now()
		}
		conn.unackedMu.Unlock()
	}
}

// HasPending returns true if there are packets waiting in the outbound queue.
func (m *TunnelManager) HasPending() bool {
	return len(m.outbound) > 0
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
		if header.Type == protocol.TunnelTypeSYN {
			conn = newICMPConn(header.SessionID, m.sender)
			conn.recvSeq = header.Seq + 1
			m.mu.Lock()
			m.sessions[header.SessionID] = conn
			m.mu.Unlock()

			// Send an ACK for SYN (reliable handshake)
			ackBytes := make([]byte, protocol.TunnelHeaderSize)
			ackHeader := protocol.TunnelHeader{
				SessionID: header.SessionID,
				Type:      protocol.TunnelTypeACK,
			}
			_ = ackHeader.Marshal(ackBytes)
			_ = m.sender(context.Background(), ackBytes)

			if m.OnSYN != nil {
				var synPayload []byte
				if header.Length > 0 && int(protocol.TunnelHeaderSize+header.Length) <= len(b) {
					synPayload = b[protocol.TunnelHeaderSize : protocol.TunnelHeaderSize+header.Length]
				}
				go m.OnSYN(conn, synPayload)
			}
			return
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

// Dial creates a new outbound ICMPConn and sends a SYN packet with the initial payload.
func (m *TunnelManager) Dial(sessionID uint32, synPayload []byte) net.Conn {
	m.mu.Lock()
	defer m.mu.Unlock()
	conn := newICMPConn(sessionID, m.sender)
	m.sessions[sessionID] = conn

	conn.unackedMu.Lock()
	hdr := protocol.TunnelHeader{
		SessionID: sessionID,
		Type:      protocol.TunnelTypeSYN,
		Seq:       0, // Use Seq 0 for SYN
		Ack:       0,
		Length:    uint16(len(synPayload)),
	}
	conn.unacked[0] = &unackedPacket{
		header:  hdr,
		payload: synPayload,
		sentAt:  time.Time{}, // Zero time: not yet sent
	}
	conn.nxtSeq = 1 // Next DATA packet will use Seq 1
	conn.unackedMu.Unlock()

	conn.send(hdr, synPayload)

	return conn
}
