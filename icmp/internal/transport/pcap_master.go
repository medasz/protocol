package transport

import (
	"context"
	"fmt"
	"net"

	"protocol/icmp/internal/protocol"

	"github.com/google/gopacket/pcap"
)

type PcapMasterResponder struct {
	SrcIP         string
	AllowedDstIPs []string
	Resolver      AddressResolver

	handle packetHandle
}

func (r *PcapMasterResponder) Serve(ctx context.Context, handler func(context.Context, protocol.RequestContext) ([]byte, error)) error {
	srcIP := net.ParseIP(r.SrcIP).To4()
	if srcIP == nil {
		return fmt.Errorf("invalid ip")
	}
	device, err := r.Resolver.ResolveDeviceByIP(srcIP)
	if err != nil {
		return err
	}
	handle, err := openLiveHandle(device, 65536, true, pcap.BlockForever)
	if err != nil {
		return err
	}
	r.handle = handle
	defer func() {
		r.handle.Close()
		r.handle = nil
	}()

	if err := handle.SetBPFFilter(BuildMasterFilter(r.AllowedDstIPs, r.SrcIP)); err != nil {
		return err
	}

	packets := handle.Packets()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case packet, ok := <-packets:
			if !ok {
				return nil
			}
			req, err := protocol.ParseEchoRequest(packet)
			if err != nil {
				return err
			}
			
			replyPayload, err := handler(ctx, req)
			if err != nil {
				return err
			}
			
			if len(replyPayload) > 0 {
				replyBytes, err := protocol.BuildEchoReply(req, replyPayload)
				if err != nil {
					return err
				}
				if err := r.handle.WritePacketData(replyBytes); err != nil {
					return err
				}
			}
		}
	}
}

// SendAsync builds an EchoReply using the provided request context and sends it immediately.
func (r *PcapMasterResponder) SendAsync(req protocol.RequestContext, payload []byte) error {
	if r.handle == nil {
		return fmt.Errorf("pcap handle not initialized")
	}
	replyBytes, err := protocol.BuildEchoReply(req, payload)
	if err != nil {
		return err
	}
	return r.handle.WritePacketData(replyBytes)
}
