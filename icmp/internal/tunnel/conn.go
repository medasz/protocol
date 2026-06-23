package tunnel

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"time"

	"protocol/icmp/internal/protocol"
)

const (
	defaultMTU       = 1000
	retransmitDelay  = 500 * time.Millisecond
	arqTickerInterval = 100 * time.Millisecond
)

type unackedPacket struct {
	header  protocol.TunnelHeader
	payload []byte
	sentAt  time.Time
}

// ICMPConn implements net.Conn over ICMP with ARQ.
type ICMPConn struct {
	sessionID uint32
	sender    SendFunc
	
	// ARQ State
	nxtSeq    uint32
	recvSeq   uint32
	unacked   map[uint32]*unackedPacket
	unackedMu sync.Mutex

	// Read buffer
	readBuf  bytes.Buffer
	readCond *sync.Cond

	// close channel
	closeCh  chan struct{}
	isClosed bool
}

func newICMPConn(sessionID uint32, sender SendFunc) *ICMPConn {
	c := &ICMPConn{
		sessionID: sessionID,
		sender:    sender,
		unacked:   make(map[uint32]*unackedPacket),
		closeCh:   make(chan struct{}),
	}
	c.readCond = sync.NewCond(&c.unackedMu) // Reusing mutex for readCond is fine
	go c.arqLoop()
	return c
}

func (c *ICMPConn) Read(b []byte) (n int, err error) {
	c.unackedMu.Lock()
	defer c.unackedMu.Unlock()

	for c.readBuf.Len() == 0 && !c.isClosed {
		c.readCond.Wait()
	}

	if c.readBuf.Len() > 0 {
		return c.readBuf.Read(b)
	}
	
	if c.isClosed {
		return 0, io.EOF
	}
	
	return 0, io.EOF
}

func (c *ICMPConn) Write(b []byte) (n int, err error) {
	if c.isClosed {
		return 0, io.ErrClosedPipe
	}

	total := len(b)
	offset := 0

	for offset < total {
		chunkSize := total - offset
		if chunkSize > defaultMTU {
			chunkSize = defaultMTU
		}

		payload := b[offset : offset+chunkSize]
		
		c.unackedMu.Lock()
		seq := c.nxtSeq
		c.nxtSeq++
		
		hdr := protocol.TunnelHeader{
			SessionID: c.sessionID,
			Type:      protocol.TunnelTypeDATA,
			Seq:       seq,
			Ack:       0, // We are not piggybacking acks in this simple impl
			Length:    uint16(chunkSize),
		}
		
		c.unacked[seq] = &unackedPacket{
			header:  hdr,
			payload: payload, // note: aliasing b, fine as long as caller doesn't modify
			sentAt:  time.Now(),
		}
		c.unackedMu.Unlock()

		c.send(hdr, payload)

		offset += chunkSize
	}

	return total, nil
}

func (c *ICMPConn) send(hdr protocol.TunnelHeader, payload []byte) {
	var buf bytes.Buffer
	// Alloc exactly 16 bytes for header
	hdrBytes := make([]byte, protocol.TunnelHeaderSize)
	hdr.Marshal(hdrBytes)
	
	buf.Write(hdrBytes)
	buf.Write(payload)
	
	// Sender must not block
	c.sender(context.Background(), buf.Bytes())
}

func (c *ICMPConn) sendAck(ack uint32) {
	hdr := protocol.TunnelHeader{
		SessionID: c.sessionID,
		Type:      protocol.TunnelTypeACK,
		Seq:       0,
		Ack:       ack,
		Length:    0,
	}
	c.send(hdr, nil)
}

func (c *ICMPConn) arqLoop() {
	ticker := time.NewTicker(arqTickerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.closeCh:
			return
		case <-ticker.C:
			c.unackedMu.Lock()
			now := time.Now()
			for _, p := range c.unacked {
				if now.Sub(p.sentAt) > retransmitDelay {
					p.sentAt = now
					c.send(p.header, p.payload)
				}
			}
			c.unackedMu.Unlock()
		}
	}
}

func (c *ICMPConn) Close() error {
	c.unackedMu.Lock()
	if c.isClosed {
		c.unackedMu.Unlock()
		return nil
	}
	c.isClosed = true
	close(c.closeCh)
	c.readCond.Broadcast() // wake up any readers
	c.unackedMu.Unlock()
	return nil
}

func (c *ICMPConn) LocalAddr() net.Addr { return &net.IPAddr{} }
func (c *ICMPConn) RemoteAddr() net.Addr { return &net.IPAddr{} }
func (c *ICMPConn) SetDeadline(t time.Time) error { return nil }
func (c *ICMPConn) SetReadDeadline(t time.Time) error { return nil }
func (c *ICMPConn) SetWriteDeadline(t time.Time) error { return nil }

func (c *ICMPConn) handleIncomingPacket(header protocol.TunnelHeader, payload []byte) {
	if header.Type == protocol.TunnelTypeACK {
		c.unackedMu.Lock()
		delete(c.unacked, header.Ack)
		c.unackedMu.Unlock()
		return
	}

	if header.Type == protocol.TunnelTypeSYN {
		// Retransmitted SYN (already handled in OnSYN but we need to ack it)
		c.sendAck(header.Seq)
	} else if header.Type == protocol.TunnelTypeDATA && len(payload) > 0 {
		c.unackedMu.Lock()
		shouldAck := false
		if header.Seq == c.recvSeq {
			// Write to buffer
			c.readBuf.Write(payload)
			c.recvSeq++
			c.readCond.Signal()
			shouldAck = true
		} else if header.Seq < c.recvSeq {
			shouldAck = true
		}
		c.unackedMu.Unlock()
		
		if shouldAck {
			// Send ACK
			c.sendAck(header.Seq)
		}
	} else if header.Type == protocol.TunnelTypeFIN {
		c.Close()
	}
}
