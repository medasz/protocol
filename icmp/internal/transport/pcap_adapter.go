package transport

import (
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/pcap"
)

type packetHandle interface {
	SetBPFFilter(string) error
	WritePacketData([]byte) error
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	Close()
	Packets() <-chan gopacket.Packet
}

type liveHandle struct {
	*pcap.Handle
}

func (h liveHandle) Packets() <-chan gopacket.Packet {
	source := gopacket.NewPacketSource(h.Handle, h.Handle.LinkType())
	return source.Packets()
}

var openLiveHandle = func(device string, snapshotLen int32, promiscuous bool, timeout time.Duration) (packetHandle, error) {
	handle, err := pcap.OpenLive(device, snapshotLen, promiscuous, timeout)
	if err != nil {
		return nil, err
	}
	return liveHandle{Handle: handle}, nil
}
