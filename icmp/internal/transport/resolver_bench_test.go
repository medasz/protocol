package transport

import "testing"

func BenchmarkParseDefaultGateway(b *testing.B) {
	out := "Active Routes:\n0.0.0.0          0.0.0.0      192.168.1.1      192.168.1.100"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ParseDefaultGateway(out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseARPTable(b *testing.B) {
	out := "Interface: 192.168.1.100 --- 0x6\n  Internet Address      Physical Address      Type\n  192.168.1.1           00-11-22-33-44-55     dynamic"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ParseARPTable(out, "192.168.1.1"); err != nil {
			b.Fatal(err)
		}
	}
}
