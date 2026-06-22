package transport

import (
	"context"
	"fmt"
	"net"

	"github.com/google/gopacket/pcap"

	"protocol/icmp/internal/protocol"
)

// TunnelListener continuously listens for incoming ProtocolTunnel packets.
type TunnelListener struct {
	TargetIP string
	Resolver AddressResolver
	handle   packetHandle
}

func (l *TunnelListener) Listen(ctx context.Context, handler func([]byte)) error {
	dstIP := net.ParseIP(l.TargetIP)
	if dstIP == nil {
		return fmt.Errorf("invalid destination ip: %s", l.TargetIP)
	}

	srcIP, err := l.Resolver.ResolveSourceIP(l.TargetIP)
	if err != nil {
		return err
	}
	device, err := l.Resolver.ResolveDeviceByIP(srcIP)
	if err != nil {
		return err
	}

	handle, err := openLiveHandle(device, 65536, true, pcap.BlockForever)
	if err != nil {
		return err
	}
	l.handle = handle
	defer func() {
		l.handle.Close()
		l.handle = nil
	}()

	if err := handle.SetBPFFilter(BuildSlaveFilter(l.TargetIP)); err != nil {
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
			
			// Slave receives Echo Replies
			ethLayer := packet.Layer(1) // Just checking if packet is valid roughly
			if ethLayer == nil { continue }
			
			// We must parse it manually since it's a reply
			// The BuildSlaveFilter ensures it's an ICMP reply from TargetIP
			reply, err := protocol.ParseEchoReply(packet.Data())
			if err != nil {
				continue
			}

			if len(reply.Payload) > 0 && reply.Payload[0] == protocol.ProtocolTunnel {
				handler(reply.Payload[1:])
			}
		}
	}
}

// SendAsync builds an EchoRequest and sends it to the target asynchronously.
func (l *TunnelListener) SendAsync(payload []byte) error {
	if l.handle == nil {
		return fmt.Errorf("TunnelListener handle not initialized")
	}

	dstIP := net.ParseIP(l.TargetIP)
	srcIP, err := l.Resolver.ResolveSourceIP(l.TargetIP)
	if err != nil {
		return err
	}
	srcMAC, err := l.Resolver.ResolveSourceMAC(srcIP)
	if err != nil {
		return err
	}
	dstMAC, err := l.Resolver.ResolveNextHopMAC(srcIP, dstIP)
	if err != nil {
		return err
	}

	requestBytes, err := protocol.BuildEchoRequest(protocol.PacketMeta{
		SrcMAC: srcMAC,
		DstMAC: dstMAC,
		SrcIP:  srcIP,
		DstIP:  dstIP,
	}, protocol.Exchange{
		ID:      1, // Dummy ID for Tunnel traffic
		Seq:     1,
		Payload: payload,
	})
	if err != nil {
		return err
	}

	return l.handle.WritePacketData(requestBytes)
}
