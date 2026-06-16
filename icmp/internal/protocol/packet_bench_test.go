package protocol

import (
	"net"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

func BenchmarkBuildEchoReply(b *testing.B) {
	req := RequestContext{
		Meta: PacketMeta{
			SrcMAC: mustMACBench(b, "00:11:22:33:44:55"),
			DstMAC: mustMACBench(b, "66:77:88:99:aa:bb"),
			SrcIP:  net.ParseIP("10.0.0.2").To4(),
			DstIP:  net.ParseIP("10.0.0.1").To4(),
		},
		Exchange: Exchange{ID: 1, Seq: 1, Payload: []byte("result")},
	}
	payload := []byte("whoami")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := BuildEchoReply(req, payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseEchoRequest(b *testing.B) {
	raw, err := BuildEchoRequest(PacketMeta{
		SrcMAC: mustMACBench(b, "00:11:22:33:44:55"),
		DstMAC: mustMACBench(b, "66:77:88:99:aa:bb"),
		SrcIP:  net.ParseIP("10.0.0.1").To4(),
		DstIP:  net.ParseIP("10.0.0.2").To4(),
	}, Exchange{ID: 1, Seq: 2, Payload: []byte("payload")})
	if err != nil {
		b.Fatal(err)
	}
	packet := gopacket.NewPacket(raw, layers.LayerTypeEthernet, gopacket.Default)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ParseEchoRequest(packet); err != nil {
			b.Fatal(err)
		}
	}
}

func mustMACBench(tb testing.TB, value string) net.HardwareAddr {
	tb.Helper()
	mac, err := net.ParseMAC(value)
	if err != nil {
		tb.Fatal(err)
	}
	return mac
}
