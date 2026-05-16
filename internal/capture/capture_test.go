package capture

import (
	"strings"
	"testing"
)

func TestBackendEncoderFallsBackToSoftware(t *testing.T) {
	b := &Backend{
		Name:       "test",
		source:     "videotestsrc",
		hwEncoders: nil,
		swEncoder:  "x264enc tune=zerolatency",
	}
	if got := b.Encoder("auto"); !strings.HasPrefix(got, "x264enc") {
		t.Fatalf("expected software fallback, got %q", got)
	}
}

func TestBackendEncoderPrefersHardware(t *testing.T) {
	b := &Backend{
		Name:   "test",
		source: "videotestsrc",
		hwEncoders: []encoderChoice{
			{"h264", "vah264enc rate-control=cbr"},
		},
		swEncoder: "x264enc tune=zerolatency",
	}
	if got := b.Encoder("auto"); !strings.HasPrefix(got, "vah264enc") {
		t.Fatalf("expected hw encoder, got %q", got)
	}
}

func TestBackendEncoderHonorsExplicitPreference(t *testing.T) {
	b := &Backend{
		Name:   "test",
		source: "videotestsrc",
		hwEncoders: []encoderChoice{
			{"h264", "vah264enc"},
			{"h265", "vah265enc"},
		},
		swEncoder: "x264enc",
	}
	if got := b.Encoder("h265"); !strings.HasPrefix(got, "vah265enc") {
		t.Fatalf("expected h265, got %q", got)
	}
	// Unknown pref → first hw encoder.
	if got := b.Encoder("av1"); !strings.HasPrefix(got, "vah264enc") {
		t.Fatalf("expected fallback to first hw, got %q", got)
	}
}

func TestLaunchStringContainsAllParts(t *testing.T) {
	b := &Backend{
		source:    "videotestsrc",
		swEncoder: "x264enc tune=zerolatency",
	}
	ls := b.LaunchString(5000, "auto")
	for _, want := range []string{"videotestsrc", "videoconvert", "x264enc", "bitrate=5000", "h264parse", "fdsink"} {
		if !strings.Contains(ls, want) {
			t.Fatalf("launch string missing %q:\n%s", want, ls)
		}
	}
}

func TestSplitArgsRespectsQuotes(t *testing.T) {
	got := splitArgs(`a "b c"  d`)
	want := []string{"a", "b c", "d"}
	if len(got) != len(want) {
		t.Fatalf("got %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] %q != %q", i, got[i], want[i])
		}
	}
}
