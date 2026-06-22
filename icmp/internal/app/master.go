package app

import (
	"context"

	"protocol/icmp/internal/protocol"
	"protocol/icmp/internal/transport"
	"protocol/icmp/internal/tunnel"
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
	Responder     transport.MasterResponder
	Commands      CommandSource
	Results       ResultSink
	Agents        AgentTracker
	TunnelManager *tunnel.TunnelManager
}

func (s MasterService) Run(ctx context.Context) error {
	return s.Responder.Serve(ctx, func(ctx context.Context, req protocol.RequestContext) ([]byte, error) {
		agentIP := req.Meta.SrcIP.String()
		if s.Agents != nil {
			s.Agents.TouchAgent(agentIP, req.Meta.SrcMAC.String())
		}
		if len(req.Exchange.Payload) == 0 {
			return nil, nil
		}

		protoID := req.Exchange.Payload[0]
		payload := req.Exchange.Payload[1:]

		if protoID == protocol.ProtocolTunnel {
			if s.TunnelManager != nil {
				s.TunnelManager.HandlePacket(payload)
			}
			return nil, nil
		}

		// Handle ProtocolShell (1)
		if len(payload) > 0 && s.Results != nil {
			if err := s.Results.WriteResult(agentIP, payload); err != nil {
				return nil, err
			}
		}
		if s.Commands == nil {
			return []byte{protocol.ProtocolShell}, nil
		}
		cmd, err := s.Commands.NextCommand(ctx, agentIP)
		if err != nil {
			return nil, err
		}
		return append([]byte{protocol.ProtocolShell}, cmd...), nil
	})
}
