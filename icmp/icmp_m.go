package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync/atomic"

	"protocol/icmp/internal/app"
	"protocol/icmp/internal/protocol"
	"protocol/icmp/internal/socks"
	"protocol/icmp/internal/stdio"
	"protocol/icmp/internal/transport"
	"protocol/icmp/internal/tunnel"
	"protocol/icmp/internal/web"
)

type masterConfig struct {
	src   string
	dst   string
	web   bool
	port  string
	shell bool
	pty   bool
	socks string
	fwd   string
}

type serviceRunner interface {
	Run(context.Context) error
}

var buildMasterRunner = func(cfg masterConfig) serviceRunner {
	var cmds app.CommandSource
	var results app.ResultSink

	if cfg.web {
		hub := web.NewHub()
		srv := web.NewServer(hub)
		go func() {
			if err := srv.Start(":" + cfg.port); err != nil {
				os.Exit(1)
			}
		}()
		cmds = hub
		results = hub
		return app.MasterService{
			Responder: &transport.PcapMasterResponder{
				SrcIP:         cfg.src,
				AllowedDstIPs: parseMasterDstAllowlist(cfg.dst),
				Resolver:      transport.OSResolver{},
			},
			Commands: cmds,
			Results:  results,
			Agents:   hub,
		}
	} else {
		cmds = nil
		results = nil
		if cfg.shell {
			cmds = stdio.NewNonBlockingCommandSource(os.Stdin)
			results = stdio.NewWriterResultSink(stdio.WrapConsoleWriter(os.Stdout))
		}
	}

	responder := &transport.PcapMasterResponder{
		SrcIP:         cfg.src,
		AllowedDstIPs: parseMasterDstAllowlist(cfg.dst),
		Resolver:      transport.OSResolver{},
	}
	
	// Create TunnelManager
	tm := tunnel.NewTunnelManager()

	var sessionID uint32 = 100 // start session IDs at 100
	if cfg.pty {
		// In a real PTY, we'd set terminal to raw mode, but for now just use os.Stdin
		go func() {
			// Small delay to ensure Slave is polling
			sid := atomic.AddUint32(&sessionID, 1)
			conn := tm.Dial(sid, []byte{protocol.CmdShell}) // dynamic session ID for PTY
			
			go func() { _, _ = io.Copy(os.Stdout, conn) }()
			_, _ = io.Copy(conn, os.Stdin)
			conn.Close()
		}()
	}
	dialer := func(target string) (net.Conn, error) {
		sid := atomic.AddUint32(&sessionID, 1)
		payload := append([]byte{protocol.CmdTCPDial}, []byte(target)...)
		conn := tm.Dial(sid, payload)
		// In a complete implementation we would wait for a success/failure signal from slave.
		// For now we assume success and return the connection.
		return conn, nil
	}

	if cfg.socks != "" {
		socksServer := socks.NewServer(":"+cfg.socks, dialer)
		go func() {
			if err := socksServer.ListenAndServe(); err != nil {
				fmt.Fprintf(os.Stderr, "SOCKS server error: %v\n", err)
			}
		}()
	}

	if cfg.fwd != "" {
		// format localPort:targetAddr:targetPort, e.g., 33890:127.0.0.1:3389
		parts := strings.SplitN(cfg.fwd, ":", 2)
		if len(parts) == 2 {
			localAddr := ":" + parts[0]
			target := parts[1]
			go func() {
				l, err := net.Listen("tcp", localAddr)
				if err != nil {
					fmt.Fprintf(os.Stderr, "FWD listen error: %v\n", err)
					return
				}
				for {
					c, err := l.Accept()
					if err != nil {
						continue
					}
					go func(conn net.Conn) {
						defer conn.Close()
						targetConn, err := dialer(target)
						if err != nil {
							return
						}
						defer targetConn.Close()
						go func() { _, _ = io.Copy(targetConn, conn) }()
						_, _ = io.Copy(conn, targetConn)
					}(c)
				}
			}()
		} else {
			fmt.Fprintf(os.Stderr, "Invalid fwd format, expected localPort:targetAddr:targetPort\n")
		}
	}

	return app.MasterService{
		Responder:     responder,
		Commands:      cmds,
		Results:       results,
		TunnelManager: tm,
	}
}

func runMaster(cfg masterConfig) error {
	if !cfg.web && len(parseMasterDstAllowlist(cfg.dst)) != 1 {
		return fmt.Errorf("master without -web requires exactly one -dst target")
	}
	return buildMasterRunner(cfg).Run(context.Background())
}
