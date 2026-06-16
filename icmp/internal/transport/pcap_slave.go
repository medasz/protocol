package transport

import (
	"context"
	"fmt"
	"net"
	"time"

	"protocol/icmp/internal/protocol"
)

type PcapPollClient struct {
	TargetIP string
	Timeout  time.Duration
	Resolver AddressResolver
	ID       uint16
	Seq      uint16
}

func (c PcapPollClient) Exchange(ctx context.Context, payload []byte) ([]byte, error) {
	dstIP := net.ParseIP(c.TargetIP)
	if dstIP == nil {
		return nil, fmt.Errorf("invalid destination ip: %s", c.TargetIP)
	}

	srcIP, err := c.Resolver.ResolveSourceIP(c.TargetIP)
	if err != nil {
		return nil, err
	}
	srcMAC, err := c.Resolver.ResolveSourceMAC(srcIP)
	if err != nil {
		return nil, err
	}
	dstMAC, err := c.Resolver.ResolveNextHopMAC(srcIP, dstIP)
	if err != nil {
		return nil, fmt.Errorf("自动获取目标 MAC 失败: %v", err)
	}
	device, err := c.Resolver.ResolveDeviceByIP(srcIP)
	if err != nil {
		return nil, err
	}

	handle, err := openLiveHandle(device, 65536, true, 100*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("打开网卡失败: %v", err)
	}
	defer handle.Close()

	requestBytes, err := protocol.BuildEchoRequest(protocol.PacketMeta{
		SrcMAC: srcMAC,
		DstMAC: dstMAC,
		SrcIP:  srcIP,
		DstIP:  dstIP,
	}, protocol.Exchange{
		ID:      c.ID,
		Seq:     c.Seq,
		Payload: payload,
	})
	if err != nil {
		return nil, fmt.Errorf("序列化 ICMP 包失败: %v", err)
	}
	if err := handle.WritePacketData(requestBytes); err != nil {
		return nil, fmt.Errorf("发送ICMP包失败: %v", err)
	}
	if err := handle.SetBPFFilter(BuildSlaveFilter(c.TargetIP)); err != nil {
		return nil, fmt.Errorf("设置BPF过滤器失败: %v", err)
	}

	deadline := time.Now().Add(c.Timeout)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout")
		}

		data, _, err := handle.ReadPacketData()
		if err != nil {
			continue
		}
		reply, err := protocol.ParseEchoReply(data)
		if err != nil {
			continue
		}
		if reply.ID == c.ID && reply.Seq == c.Seq {
			return reply.Payload, nil
		}
	}
}
