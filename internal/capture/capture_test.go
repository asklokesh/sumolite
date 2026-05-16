package capture

import (
	"strings"
	"testing"
)

func TestDetectReturnsBackendForHost(t *testing.T) {
	b, err := Detect()
	if err != nil {
		t.Fatal(err)
	}
	if b.program == "" {
		t.Fatal("backend missing program")
	}
	if b.argsFor == nil {
		t.Fatal("backend missing argsFor")
	}
	args := b.argsFor(8000, "auto")
	if len(args) == 0 {
		t.Fatal("argsFor returned no args")
	}
}

func TestUseTestSourceFallsBackToGStreamer(t *testing.T) {
	b, err := Detect()
	if err != nil {
		t.Fatal(err)
	}
	b.UseTestSource()
	if b.program != "gst-launch-1.0" {
		t.Fatalf("test source should use gst-launch-1.0, got %q", b.program)
	}
	args := strings.Join(b.argsFor(5000, "auto"), " ")
	for _, want := range []string{"videotestsrc", "x264enc", "bitrate=5000", "h264parse", "fdsink"} {
		if !strings.Contains(args, want) {
			t.Fatalf("test source args missing %q in:\n%s", want, args)
		}
	}
}

func TestDarwinBackendShape(t *testing.T) {
	b := darwinFFmpeg()
	if b.program != "ffmpeg" {
		t.Fatalf("darwin should use ffmpeg, got %q", b.program)
	}
	args := b.argsFor(8000, "auto")
	got := strings.Join(args, " ")
	for _, want := range []string{"avfoundation", "h264_videotoolbox", "8000k", "-f", "h264", "pipe:1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("darwin args missing %q in:\n%s", want, got)
		}
	}
}

func TestLinuxBackendShape(t *testing.T) {
	b := linuxGStreamer()
	if b.program != "gst-launch-1.0" {
		t.Fatalf("linux should use gst-launch-1.0, got %q", b.program)
	}
	args := strings.Join(b.argsFor(7000, "auto"), " ")
	for _, want := range []string{"videoconvert", "bitrate=7000", "h264parse", "fdsink"} {
		if !strings.Contains(args, want) {
			t.Fatalf("linux args missing %q in:\n%s", want, args)
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
