package app

import (
	"context"

	"protocol/icmp/internal/protocol"
	"protocol/icmp/internal/transport"
)

type CommandSource interface {
	NextCommand(context.Context) ([]byte, error)
}

type ResultSink interface {
	WriteResult([]byte) error
}

type MasterService struct {
	Responder transport.MasterResponder
	Commands  CommandSource
	Results   ResultSink
}

func (s MasterService) Run(ctx context.Context) error {
	return s.Responder.Serve(ctx, func(ctx context.Context, req protocol.Exchange) ([]byte, error) {
		if len(req.Payload) > 0 && s.Results != nil {
			if err := s.Results.WriteResult(req.Payload); err != nil {
				return nil, err
			}
		}
		if s.Commands == nil {
			return nil, nil
		}
		return s.Commands.NextCommand(ctx)
	})
}
