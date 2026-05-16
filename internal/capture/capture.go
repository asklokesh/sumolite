// Package capture picks the right GStreamer source + hardware encoder for the
// host, then exposes a launch string that produces RTP H.264/H.265 ready to
// hand to Pion.
package capture

import (
	"fmt"
	"os/exec"
	"runtime"
)

type Backend struct {
	Name       string
	source     string
	hwEncoders []encoderChoice
	swEncoder  string
}

type encoderChoice struct {
	codec string // h264 | h265 | av1
	gst   string // gstreamer element + props
}

// Detect inspects the host and returns the best available capture backend.
// It does not fail if hardware encode is missing — the software fallback is
// always last in line.
func Detect() (*Backend, error) {
	switch runtime.GOOS {
	case "darwin":
		return darwin(), nil
	case "linux":
		return linuxBackend(), nil
	default:
		return nil, fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

func darwin() *Backend {
	return &Backend{
		Name:   "macOS / avfvideosrc",
		source: "avfvideosrc capture-screen=true capture-screen-cursor=true",
		hwEncoders: []encoderChoice{
			{"h264", "vtenc_h264_hw realtime=true allow-frame-reordering=false max-keyframe-interval=240"},
			{"h265", "vtenc_h265_hw realtime=true allow-frame-reordering=false max-keyframe-interval=240"},
		},
		swEncoder: "x264enc tune=zerolatency speed-preset=ultrafast key-int-max=240",
	}
}

func linuxBackend() *Backend {
	source := "ximagesrc use-damage=0 show-pointer=true"
	if hasElement("pipewiresrc") {
		// Wayland / portal path
		source = "pipewiresrc do-timestamp=true"
	}
	enc := []encoderChoice{}
	if hasElement("nvh264enc") {
		enc = append(enc, encoderChoice{"h264", "nvh264enc preset=low-latency-hp rc-mode=cbr gop-size=240"})
	}
	if hasElement("vah264enc") {
		enc = append(enc, encoderChoice{"h264", "vah264enc rate-control=cbr key-int-max=240"})
	}
	if hasElement("vaapih264enc") {
		enc = append(enc, encoderChoice{"h264", "vaapih264enc rate-control=cbr keyframe-period=240"})
	}
	return &Backend{
		Name:       "linux / " + source,
		source:     source,
		hwEncoders: enc,
		swEncoder:  "x264enc tune=zerolatency speed-preset=ultrafast key-int-max=240",
	}
}

// Encoder picks an encoder element string, honoring the user's preference
// ("auto" picks the first hardware encoder, falling back to software).
func (b *Backend) Encoder(pref string) string {
	if pref != "auto" {
		for _, e := range b.hwEncoders {
			if e.codec == pref {
				return e.gst
			}
		}
	}
	if len(b.hwEncoders) > 0 {
		return b.hwEncoders[0].gst
	}
	return b.swEncoder
}

// LaunchString builds a full gst-launch pipeline that produces raw H.264
// NAL units on stdout, ready to be packetized into RTP by Pion.
//
// We deliberately don't muxbecause Pion handles RTP packetization itself.
func (b *Backend) LaunchString(bitrateKbps int, pref string) string {
	enc := b.Encoder(pref)
	return fmt.Sprintf(
		"%s ! videoconvert ! video/x-raw,format=NV12 ! %s bitrate=%d ! h264parse config-interval=-1 ! fdsink",
		b.source, enc, bitrateKbps,
	)
}

func hasElement(name string) bool {
	return exec.Command("gst-inspect-1.0", name).Run() == nil
}
