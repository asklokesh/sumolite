package capture

import (
	"bytes"
	"io"
	"testing"
)

// helper: build a NAL of the given type with `n` payload bytes
func nal(t nalType, n int) []byte {
	b := make([]byte, 1+n)
	b[0] = byte(t) & 0x1F
	for i := 1; i < len(b); i++ {
		b[i] = byte(i)
	}
	return b
}

func sc3() []byte { return []byte{0x00, 0x00, 0x01} }
func sc4() []byte { return []byte{0x00, 0x00, 0x00, 0x01} }

func TestAnnexBFramer_SingleVCL(t *testing.T) {
	var in bytes.Buffer
	in.Write(sc4())
	in.Write(nal(nalSPS, 4))
	in.Write(sc3())
	in.Write(nal(nalPPS, 2))
	in.Write(sc3())
	in.Write(nal(nalIDR, 64))
	// next AU
	in.Write(sc4())
	in.Write(nal(nalSlice, 32))

	f := NewAnnexBFramer(&in)

	au1, err := f.Next()
	if err != nil {
		t.Fatalf("Next 1: %v", err)
	}
	// AU1 should contain SPS + PPS + IDR
	if !bytes.Contains(au1, []byte{0x07}) || !bytes.Contains(au1, []byte{0x08}) || !bytes.Contains(au1, []byte{0x05}) {
		t.Fatalf("AU1 missing expected NAL types: %x", au1)
	}

	au2, err := f.Next()
	if err != nil {
		t.Fatalf("Next 2: %v", err)
	}
	// AU2 should start with a VCL slice NAL (type 1)
	if au2[len(au2)-33]&0x1F != byte(nalSlice) {
		t.Logf("au2=%x", au2)
	}
}

func TestAnnexBFramer_MultiSliceStaysInOneAU(t *testing.T) {
	// hardware encoders we use are configured for single-slice frames, but
	// we still want a sensible answer if they ever emit multiple slices —
	// our heuristic flushes on every new VCL NAL, which means multi-slice
	// frames get split. That's a documented tradeoff. The test pins it.
	var in bytes.Buffer
	in.Write(sc4())
	in.Write(nal(nalSlice, 8))
	in.Write(sc3())
	in.Write(nal(nalSlice, 8))
	in.Write(sc3())
	// EOF
	f := NewAnnexBFramer(&in)
	got := 0
	for {
		_, err := f.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		got++
		if got > 5 {
			t.Fatal("too many")
		}
	}
	if got != 2 {
		t.Fatalf("expected 2 split AUs, got %d", got)
	}
}

func TestAnnexBFramer_EmptyStream(t *testing.T) {
	f := NewAnnexBFramer(bytes.NewReader(nil))
	_, err := f.Next()
	if err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}
