// Package capture picks the right screen-capture + hardware-encoder
// pipeline for the host. On Linux it uses GStreamer. On macOS it uses
// ffmpeg because gst-launch's avfvideosrc screen capture is broken on
// Apple Silicon (caps negotiation fails since Apple deprecated the old
// CGDisplayStream API). ffmpeg's avfoundation indev still works and the
// h264_videotoolbox encoder gives us hardware H.264 with one fewer
// subprocess to manage.
package capture

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Backend is a platform-specific capture+encode plan. Start() spawns the
// chosen command and the framer reads Annex-B H.264 off its stdout.
type Backend struct {
	Name string

	// program is the binary to exec ("gst-launch-1.0" or "ffmpeg").
	program string

	// argsFor returns the full argv given the runtime knobs.
	argsFor func(bitrateKbps int, pref string) []string

	// For introspection / tests.
	encoderPref string
}

// Detect inspects the host and returns the best available capture backend.
func Detect() (*Backend, error) {
	switch runtime.GOOS {
	case "darwin":
		return darwinFFmpeg(), nil
	case "linux":
		return linuxGStreamer(), nil
	default:
		return nil, fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

// UseTestSource swaps the platform screen capture for a synthetic moving
// pattern, validating the encode and WebRTC paths without requiring
// Screen Recording permission.
func (b *Backend) UseTestSource() {
	b.program = "gst-launch-1.0"
	b.argsFor = func(bitrateKbps int, pref string) []string {
		enc := "x264enc tune=zerolatency speed-preset=ultrafast bitrate=" + itoa(bitrateKbps) + " key-int-max=60"
		pipeline := "videotestsrc is-live=true pattern=ball ! video/x-raw,width=1280,height=720,framerate=60/1 ! videoconvert ! video/x-raw,format=I420 ! " + enc + " ! h264parse config-interval=-1 ! fdsink"
		return append([]string{"-q"}, splitArgs(pipeline)...)
	}
	b.Name = "test source (videotestsrc + x264)"
}

// Encoder returns a human label for the configured encoder. Used for
// logging at startup so the operator can see which hardware path was
// chosen.
func (b *Backend) Encoder(pref string) string { return b.encoderPref }

// DisplayIndex picks which AVFoundation screen device to capture. On
// macOS device 0 is usually the FaceTime camera, screen 0 is device 1,
// screen 1 is device 2, etc. `sumolite doctor` lists what's available.
var DisplayIndex = "1"

// darwinFFmpeg drives ffmpeg with AVFoundation screen capture and the
// h264_videotoolbox hardware encoder, writing raw Annex-B H.264 to
// stdout. We pick the first screen device (index 1 on most setups;
// device 0 is the FaceTime camera). The framerate, GOP and bitrate are
// tuned for low-latency streaming.
func darwinFFmpeg() *Backend {
	return &Backend{
		Name:        "macOS / ffmpeg avfoundation + h264_videotoolbox",
		program:     "ffmpeg",
		encoderPref: "h264_videotoolbox",
		argsFor: func(bitrateKbps int, pref string) []string {
			return []string{
				"-hide_banner", "-loglevel", "error", "-nostdin",
				"-f", "avfoundation",
				"-capture_cursor", "1",
				"-framerate", "60",
				"-pixel_format", "nv12",
				"-i", DisplayIndex + ":none", // selected screen, no audio
				// fps=60 in the filter graph forces the decimator to
				// drop duplicated frames AVFoundation emits when the
				// screen is idle, and -r 60 caps output framerate so
				// h264_videotoolbox doesn't encode the same frame twice.
				// Without this AVFoundation pushes frames at ~200fps
				// regardless of the -framerate hint, the encoder
				// produces 200 small frames per second, and the
				// decoder on the receiving end gets RTP timestamps
				// that advance 3x faster than real time and freezes
				// after the first frame.
				"-vf", "scale=-2:1080,fps=60",
				"-r", "60",
				"-c:v", "h264_videotoolbox",
				"-realtime", "1",
				"-allow_sw", "0",
				"-b:v", itoa(bitrateKbps) + "k",
				"-g", "120",
				"-pix_fmt", "nv12",
				"-f", "h264",
				"pipe:1",
			}
		},
	}
}

func linuxGStreamer() *Backend {
	source := "ximagesrc use-damage=0 show-pointer=true"
	if hasElement("pipewiresrc") {
		source = "pipewiresrc do-timestamp=true"
	}
	enc := "x264enc tune=zerolatency speed-preset=ultrafast key-int-max=240"
	encName := "x264enc (software)"
	switch {
	case hasElement("nvh264enc"):
		enc = "nvh264enc preset=low-latency-hp rc-mode=cbr gop-size=240"
		encName = "nvh264enc (NVIDIA)"
	case hasElement("vah264enc"):
		enc = "vah264enc rate-control=cbr key-int-max=240"
		encName = "vah264enc (VA-API)"
	case hasElement("vaapih264enc"):
		enc = "vaapih264enc rate-control=cbr keyframe-period=240"
		encName = "vaapih264enc (VA-API legacy)"
	}
	return &Backend{
		Name:        "linux / " + source,
		program:     "gst-launch-1.0",
		encoderPref: encName,
		argsFor: func(bitrateKbps int, pref string) []string {
			pipeline := fmt.Sprintf(
				"%s ! videoconvert ! video/x-raw,format=NV12 ! %s bitrate=%d ! h264parse config-interval=-1 ! fdsink",
				source, enc, bitrateKbps,
			)
			return append([]string{"-q"}, splitArgs(pipeline)...)
		},
	}
}

func hasElement(name string) bool {
	return exec.Command("gst-inspect-1.0", name).Run() == nil
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
