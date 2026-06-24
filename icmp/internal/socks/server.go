package socks

import (
	"encoding/binary"
	"io"
	"net"
	"strconv"
)

// Dialer is a function that connects to a target and returns a net.Conn.
type Dialer func(target string) (net.Conn, error)

// Server is a simple SOCKS5 server that uses a custom dialer to route traffic.
type Server struct {
	addr   string
	dialer Dialer
}

// NewServer creates a new SOCKS5 server.
func NewServer(addr string, dialer Dialer) *Server {
	return &Server{
		addr:   addr,
		dialer: dialer,
	}
}

// Start starts the SOCKS5 server on the provided listener.
func (s *Server) Start(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		go s.handleConnection(conn)
	}
}

// ListenAndServe starts the SOCKS5 server and blocks until an error occurs.
func (s *Server) ListenAndServe() error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	defer l.Close()
	s.Start(l)
	return nil
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 256)

	// 1. Handshake
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	if buf[0] != 0x05 { // Not Socks5
		return
	}
	nmethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:nmethods]); err != nil {
		return
	}
	// Reply NO AUTH (0x00)
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// 2. Read Connect Request
	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return
	}
	if buf[0] != 0x05 || buf[1] != 0x01 || buf[2] != 0x00 { // We only support CONNECT (0x01)
		// Reply Command not supported (0x07)
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	var targetAddr string
	atyp := buf[3]
	switch atyp {
	case 0x01: // IPv4
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return
		}
		targetAddr = net.IP(buf[:4]).String()
	case 0x03: // Domain name
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return
		}
		domainLen := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:domainLen]); err != nil {
			return
		}
		targetAddr = string(buf[:domainLen])
	case 0x04: // IPv6
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return
		}
		targetAddr = net.IP(buf[:16]).String()
	default:
		// Reply Address type not supported (0x08)
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	targetPort := binary.BigEndian.Uint16(buf[:2])
	target := net.JoinHostPort(targetAddr, strconv.Itoa(int(targetPort)))

	// 3. Dial via our TunnelManager / Dialer
	targetConn, err := s.dialer(target)
	if err != nil {
		// Reply Host unreachable (0x04)
		conn.Write([]byte{0x05, 0x04, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer targetConn.Close()

	// Reply Success (0x00)
	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})

	// 4. Forwarding traffic
	go func() {
		_, _ = io.Copy(targetConn, conn)
	}()
	_, _ = io.Copy(conn, targetConn)
}
