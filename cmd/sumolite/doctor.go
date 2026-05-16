package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

func runDoctor() {
	switch runtime.GOOS {
	case "darwin":
		check("ffmpeg", "-version")
		// ffmpeg from Homebrew bundles avfoundation + videotoolbox on
		// Apple Silicon, but a custom build might not. List the encoders
		// so the user can see h264_videotoolbox in the output.
		check("ffmpeg", "-hide_banner", "-encoders")
		fmt.Println()
		fmt.Println("  macOS note: grant Screen Recording permission to /opt/homebrew/bin/ffmpeg")
		fmt.Println("  in System Settings -> Privacy & Security -> Screen & System Audio Recording")
	case "linux":
		check("gst-launch-1.0", "--version")
		check("gst-inspect-1.0", "pipewiresrc")
		any := tryAny([][]string{
			{"gst-inspect-1.0", "vah264enc"},
			{"gst-inspect-1.0", "nvh264enc"},
			{"gst-inspect-1.0", "vaapih264enc"},
		})
		if !any {
			fmt.Println("  WARN: no hardware H.264 encoder found; software fallback will be used")
		}
	}
}

func check(name string, args ...string) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("  FAIL %s %v: %v\n%s\n", name, args, err, out)
		return
	}
	// keep the encoder list short
	if len(out) > 400 {
		out = out[:400]
	}
	fmt.Printf("  OK   %s %v\n", name, args)
	_ = out
}

func tryAny(cmds [][]string) bool {
	for _, c := range cmds {
		if err := exec.Command(c[0], c[1:]...).Run(); err == nil {
			fmt.Printf("  OK   %v\n", c)
			return true
		}
	}
	return false
}
