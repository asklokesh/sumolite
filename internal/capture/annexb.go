package capture

import (
	"bufio"
	"errors"
	"io"
)

// Annex-B NAL unit framer.
//
// The H.264 elementary stream we get out of `h264parse ! fdsink` is a
// concatenation of NAL units, each prefixed with a 3- or 4-byte start code
// (0x000001 / 0x00000001). For WebRTC we want to feed Pion one access
// unit (== one decoded frame) per media.Sample, so we:
//
//   1. Stream bytes through a rolling 3-byte window to find start codes.
//   2. Split out NAL units.
//   3. Group consecutive NALs into access units, flushing when we see the
//      next AU boundary (an AUD, an SPS, or a VCL NAL after a prior VCL
//      NAL in the same buffer).
//
// The output retains the start codes so Pion's H264 packetizer can locate
// NAL boundaries itself.

type nalType uint8

const (
	nalSlice nalType = 1
	nalIDR   nalType = 5
	nalSEI   nalType = 6
	nalSPS   nalType = 7
	nalPPS   nalType = 8
	nalAUD   nalType = 9
)

func isVCL(t nalType) bool { return t >= 1 && t <= 5 }

// AnnexBFramer reads from an io.Reader and emits whole access units.
type AnnexBFramer struct {
	r      *bufio.Reader
	buf    []byte // unflushed NAL bytes for the in-progress access unit
	hadVCL bool
}

func NewAnnexBFramer(r io.Reader) *AnnexBFramer {
	return &AnnexBFramer{r: bufio.NewReaderSize(r, 1<<20)}
}

// Next blocks until the next complete access unit is available and returns
// its bytes (with start codes). The returned slice is owned by the caller —
// the framer reuses no internal buffers across calls.
func (f *AnnexBFramer) Next() ([]byte, error) {
	// We accumulate NAL units until we encounter the start of the *next*
	// access unit, then return what we have. To stream cleanly we read
	// byte-by-byte through a small lookahead.
	//
	// Wire format we expect from gstreamer's h264parse:
	//   [start_code] [NAL1] [start_code] [NAL2] ...
	//
	// We don't try to decode slice headers; the AU boundary heuristic is:
	//   - AUD (type 9), SPS (7), or PPS (8) always starts a new AU.
	//   - Any VCL NAL after we've already seen a VCL NAL in this AU also
	//     starts a new one (multi-slice frames stay in the same AU because
	//     we don't actually peek at first_mb_in_slice; this is a
	//     deliberate simplification for hardware-encoded single-slice
	//     frames, which is what our pipelines produce).

	for {
		// Find next start code.
		sc, err := f.readToStartCode()
		if err != nil {
			if errors.Is(err, io.EOF) && len(f.buf) > 0 {
				out := f.buf
				f.buf, f.hadVCL = nil, false
				return out, nil
			}
			return nil, err
		}

		// sc is the start-code prefix bytes that we just consumed.
		// Peek the NAL header byte (forbidden_zero_bit + nal_ref_idc + nal_unit_type).
		hdr, err := f.r.ReadByte()
		if err != nil {
			// A trailing start code with no NAL after it: discard the
			// dangling start code and flush whatever AU we have buffered.
			if errors.Is(err, io.EOF) && len(f.buf) > 0 {
				out := f.buf
				f.buf, f.hadVCL = nil, false
				return out, nil
			}
			return nil, err
		}
		t := nalType(hdr & 0x1F)

		// A new access unit starts when:
		//   - We see an AUD (always marks the start of an AU).
		//   - We see SPS/PPS/SEI AFTER we've already collected a VCL NAL
		//     in this AU (parameter sets prefixing the NEXT frame).
		//   - We see any VCL NAL AFTER a previous VCL (multi-slice frames
		//     stay together in practice because our encoder pipelines
		//     emit single-slice frames; treating any VCL-after-VCL as a
		//     boundary is the correct single-slice assumption).
		//
		// Previously we flushed on every SPS, which split each IDR's
		// parameter-set prefix from its slice and made every frame
		// emit ~3 sub-AUs. That tripled RTP timestamps and stalled
		// the decoder after the first frame.
		boundary := false
		switch {
		case t == nalAUD:
			boundary = true
		case (t == nalSPS || t == nalPPS || t == nalSEI) && f.hadVCL:
			boundary = true
		case isVCL(t) && f.hadVCL:
			boundary = true
		}

		if boundary && len(f.buf) > 0 {
			out := f.buf
			f.buf = append([]byte{}, sc...)
			f.buf = append(f.buf, hdr)
			f.hadVCL = isVCL(t)
			return out, nil
		}

		f.buf = append(f.buf, sc...)
		f.buf = append(f.buf, hdr)
		if isVCL(t) {
			f.hadVCL = true
		}
	}
}

// readToStartCode reads from f.r into f.buf until it has consumed all
// bytes up to and including the next start-code prefix. It returns just
// the start-code bytes that were consumed. Bytes that belong to the
// previous NAL are appended to f.buf.
func (f *AnnexBFramer) readToStartCode() ([]byte, error) {
	// Rolling 3-byte window. State 0 = empty, 1 = saw 0x00, 2 = saw 0x00 0x00,
	// 3+ = saw 0x00 0x00 ... ; once we see a final 0x01 with at least two
	// preceding zeroes, that's a start code.
	zeros := 0
	for {
		b, err := f.r.ReadByte()
		if err != nil {
			return nil, err
		}
		switch {
		case b == 0x00:
			zeros++
			f.buf = append(f.buf, b)
		case b == 0x01 && zeros >= 2:
			// Trim the trailing 0x00 bytes we appended into f.buf and
			// return them + the 0x01 as the start code prefix.
			n := zeros
			if n > 3 {
				n = 3 // standard allows 4-byte start code (000001 preceded by extra zero)
			}
			f.buf = f.buf[:len(f.buf)-n]
			sc := make([]byte, n+1)
			for i := 0; i < n; i++ {
				sc[i] = 0x00
			}
			sc[n] = 0x01
			return sc, nil
		default:
			zeros = 0
			f.buf = append(f.buf, b)
		}
	}
}
