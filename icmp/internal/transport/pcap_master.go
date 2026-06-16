package transport

import (
	"context"
	"fmt"
	"net"

	"protocol/icmp/internal/protocol"

	"github.com/google/gopacket/pcap"
)

type PcapMasterResponder struct {
	SrcIP    string
	DstIP    string
	Resolver AddressResolver
}

func (r PcapMasterResponder) Serve(ctx context.Context, handler func(context.Context, protocol.Exchange) ([]byte, error)) error {
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
	defer handle.Close()

	if err := handle.SetBPFFilter(BuildMasterFilter(r.DstIP, r.SrcIP)); err != nil {
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
			replyPayload, err := handler(ctx, req.Exchange)
			if err != nil {
				return err
			}
			replyBytes, err := protocol.BuildEchoReply(req, replyPayload)
			if err != nil {
				return err
			}
			if err := handle.WritePacketData(replyBytes); err != nil {
				return err
			}
		}
	}
}
