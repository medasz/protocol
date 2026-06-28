package tunnel

import (
	"context"
	"net"
	"testing"
	"time"

	"protocol/icmp/internal/protocol"
)

func TestICMPConn_OutOfOrderDelivery(t *testing.T) {
	tmA := NewTunnelManager()
	tmB := NewTunnelManager()

	// A's sender puts packet into B
	// But we intercept it to reorder.
	interceptCh := make(chan []byte, 100)
	
	// Override tmA's sender for intercepting
	tmA.sender = func(ctx context.Context, payload []byte) error {
		interceptCh <- payload
		return nil
	}
	
	// B's sender puts packet into A (for ACKs)
	tmB.sender = func(ctx context.Context, payload []byte) error {
		tmA.HandlePacket(payload)
		return nil
	}

	sessionID := uint32(100)
	synPayload := []byte("SYN")
	
	// Setup B's OnSYN
	var bConn *ICMPConn
	synWait := make(chan struct{})
	tmB.OnSYN = func(conn net.Conn, payload []byte) {
		bConn = conn.(*ICMPConn)
		close(synWait)
	}

	// 1. A dials B
	aConn := tmA.Dial(sessionID, synPayload)
	
	// Deliver SYN
	synPkt := <-interceptCh
	tmB.HandlePacket(synPkt)
	
	<-synWait
	if bConn == nil {
		t.Fatal("B did not receive SYN")
	}

	// 2. A writes 3 chunks of data
	go func() {
		aConn.Write([]byte("Chunk1"))
		aConn.Write([]byte("Chunk2"))
		aConn.Write([]byte("Chunk3"))
	}()

	// 3. Intercept the 3 chunks
	var packets [][]byte
	for i := 0; i < 3; i++ {
		p := <-interceptCh
		packets = append(packets, p)
	}

	// Verify they are DATA packets
	for _, p := range packets {
		hdr, _ := protocol.UnmarshalTunnelHeader(p)
		if hdr.Type != protocol.TunnelTypeDATA {
			t.Fatalf("Expected DATA packet, got %v", hdr.Type)
		}
	}

	// 4. Deliver them out of order: 3, 2, 1
	// Sequence numbers for data will be 1, 2, 3 (since SYN is 0)
	tmB.HandlePacket(packets[2]) // Chunk 3
	
	// Let B process
	time.Sleep(50 * time.Millisecond)
	
	// B's read buffer should be empty because it's missing Chunk 1 and 2
	bConn.unackedMu.Lock()
	if bConn.readBuf.Len() != 0 {
		t.Fatalf("Expected readBuf to be empty, got %d", bConn.readBuf.Len())
	}
	bConn.unackedMu.Unlock()

	tmB.HandlePacket(packets[1]) // Chunk 2
	time.Sleep(50 * time.Millisecond)

	// Still empty
	bConn.unackedMu.Lock()
	if bConn.readBuf.Len() != 0 {
		t.Fatalf("Expected readBuf to be empty, got %d", bConn.readBuf.Len())
	}
	bConn.unackedMu.Unlock()

	// Deliver Chunk 1
	tmB.HandlePacket(packets[0]) // Chunk 1
	time.Sleep(50 * time.Millisecond)

	// Now it should have all 3 chunks in order!
	readOut := make([]byte, 100)
	n, err := bConn.Read(readOut)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	
	result := string(readOut[:n])
	expected := "Chunk1Chunk2Chunk3"
	if result != expected {
		t.Fatalf("Expected '%s', got '%s'", expected, result)
	}
}
