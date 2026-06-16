package protocol

import (
	"net"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

func TestBuildEchoReplyRoundTrip(t *testing.T) {
	req := RequestContext{
		Meta: PacketMeta{
			SrcMAC: mustMAC(t, "00:11:22:33:44:55"),
			DstMAC: mustMAC(t, "66:77:88:99:aa:bb"),
			SrcIP:  net.ParseIP("10.0.0.2").To4(),
			DstIP:  net.ParseIP("10.0.0.1").To4(),
		},
		Exchange: Exchange{
			ID:      7,
			Seq:     9,
			Payload: []byte("result"),
		},
	}

	replyBytes, err := BuildEchoReply(req, []byte("whoami"))
	if err != nil {
		t.Fatalf("BuildEchoReply() error = %v", err)
	}

	packet := gopacket.NewPacket(replyBytes, layers.LayerTypeEthernet, gopacket.Default)
	eth := packet.Layer(layers.LayerTypeEthernet).(*layers.Ethernet)
	ip := packet.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	icmp := packet.Layer(layers.LayerTypeICMPv4).(*layers.ICMPv4)

	if got, want := eth.SrcMAC.String(), req.Meta.DstMAC.String(); got != want {
		t.Fatalf("reply src mac = %s, want %s", got, want)
	}
	if got, want := eth.DstMAC.String(), req.Meta.SrcMAC.String(); got != want {
		t.Fatalf("reply dst mac = %s, want %s", got, want)
	}
	if !ip.SrcIP.Equal(req.Meta.DstIP) || !ip.DstIP.Equal(req.Meta.SrcIP) {
		t.Fatalf("unexpected ip swap: src=%s dst=%s", ip.SrcIP, ip.DstIP)
	}
	if icmp.Id != req.Exchange.ID || icmp.Seq != req.Exchange.Seq {
		t.Fatalf("unexpected echo tuple: id=%d seq=%d", icmp.Id, icmp.Seq)
	}
	if got, want := string(icmp.Payload), "whoami"; got != want {
		t.Fatalf("reply payload = %q, want %q", got, want)
	}
}

func TestBuildEchoRequestAndParseReply(t *testing.T) {
	meta := PacketMeta{
		SrcMAC: mustMAC(t, "00:11:22:33:44:55"),
		DstMAC: mustMAC(t, "66:77:88:99:aa:bb"),
		SrcIP:  net.ParseIP("10.0.0.1").To4(),
		DstIP:  net.ParseIP("10.0.0.2").To4(),
	}
	msg := Exchange{ID: 1, Seq: 2, Payload: []byte("payload")}

	raw, err := BuildEchoRequest(meta, msg)
	if err != nil {
		t.Fatalf("BuildEchoRequest() error = %v", err)
	}

	parsed, err := ParseEchoReply(raw)
	if err != nil {
		t.Fatalf("ParseEchoReply() error = %v", err)
	}
	if parsed.ID != msg.ID || parsed.Seq != msg.Seq || string(parsed.Payload) != string(msg.Payload) {
		t.Fatalf("unexpected parsed reply: %+v", parsed)
	}
}

func TestParseEchoRequest(t *testing.T) {
	raw, err := BuildEchoRequest(PacketMeta{
		SrcMAC: mustMAC(t, "00:11:22:33:44:55"),
		DstMAC: mustMAC(t, "66:77:88:99:aa:bb"),
		SrcIP:  net.ParseIP("10.1.1.1").To4(),
		DstIP:  net.ParseIP("10.1.1.2").To4(),
	}, Exchange{ID: 11, Seq: 22, Payload: []byte("hello")})
	if err != nil {
		t.Fatalf("BuildEchoRequest() error = %v", err)
	}

	packet := gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.Default)
	req, err := ParseEchoRequest(packet)
	if err != nil {
		t.Fatalf("ParseEchoRequest() error = %v", err)
	}
	if got, want := req.Exchange.ID, uint16(11); got != want {
		t.Fatalf("id = %d, want %d", got, want)
	}
	if got, want := req.Exchange.Seq, uint16(22); got != want {
		t.Fatalf("seq = %d, want %d", got, want)
	}
	if got, want := string(req.Exchange.Payload), "hello"; got != want {
		t.Fatalf("payload = %q, want %q", got, want)
	}
}

func TestParseEchoRequestMissingICMP(t *testing.T) {
	buffer := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true},
		&layers.Ethernet{
			SrcMAC:       mustMAC(t, "00:11:22:33:44:55"),
			DstMAC:       mustMAC(t, "66:77:88:99:aa:bb"),
			EthernetType: layers.EthernetTypeIPv4,
		},
		&layers.IPv4{
			SrcIP:    net.ParseIP("10.0.0.1").To4(),
			DstIP:    net.ParseIP("10.0.0.2").To4(),
			Protocol: layers.IPProtocolTCP,
			Version:  4,
			TTL:      64,
		},
	); err != nil {
		t.Fatalf("SerializeLayers() error = %v", err)
	}

	packet := gopacket.NewPacket(buffer.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
	_, err := ParseEchoRequest(packet)
	if err != ErrMissingICMPv4 {
		t.Fatalf("ParseEchoRequest() error = %v, want %v", err, ErrMissingICMPv4)
	}
}

func mustMAC(t *testing.T, value string) net.HardwareAddr {
	t.Helper()
	mac, err := net.ParseMAC(value)
	if err != nil {
		t.Fatalf("ParseMAC(%q) error = %v", value, err)
	}
	return mac
}
