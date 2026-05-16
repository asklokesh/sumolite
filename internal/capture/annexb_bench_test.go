package capture

import (
	"bytes"
	"testing"
)

// BenchmarkAnnexBFramer measures the throughput of the framer on a
// realistic-shape stream: SPS/PPS/IDR followed by 119 P-frames of 8 KB
// each (~6 Mbps at 120fps). On an M1 the framer should sit well under
// 1µs per AU, leaving plenty of headroom over the encoder.
func BenchmarkAnnexBFramer(b *testing.B) {
	var buf bytes.Buffer
	// IDR + parameter sets up front.
	buf.Write(sc4())
	buf.Write(nal(nalSPS, 16))
	buf.Write(sc3())
	buf.Write(nal(nalPPS, 8))
	buf.Write(sc3())
	buf.Write(nal(nalIDR, 8192))
	for i := 0; i < 119; i++ {
		buf.Write(sc3())
		buf.Write(nal(nalSlice, 8192))
	}
	data := buf.Bytes()

	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := NewAnnexBFramer(bytes.NewReader(data))
		for {
			if _, err := f.Next(); err != nil {
				break
			}
		}
	}
}
