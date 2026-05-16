package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

func runDoctor() {
	switch runtime.GOOS {
	case "darwin":
		check("ffmpeg", "-version")
		listDarwinDisplays()
		fmt.Println()
		fmt.Println("  macOS note: grant Screen Recording permission to /opt/homebrew/bin/ffmpeg")
		fmt.Println("  in System Settings -> Privacy & Security -> Screen & System Audio Recording")
		fmt.Println("  Use --display <index> to pick a specific screen from the list above.")
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

// listDarwinDisplays asks ffmpeg to print AVFoundation devices and pulls
// out the "Capture screen" entries so the user can see what indices are
// available to pass to --display.
func listDarwinDisplays() {
	cmd := exec.Command("ffmpeg", "-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", "")
	out, _ := cmd.CombinedOutput()
	fmt.Println("\n  AVFoundation devices (use index N with --display N):")
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "AVFoundation video devices") ||
			strings.Contains(line, "Capture screen") ||
			(strings.Contains(line, "Camera") && strings.Contains(line, "]")) {
			// strip the leading "[AVFoundation indev @ 0x...]" prefix
			if i := strings.Index(line, "] "); i >= 0 {
				line = line[i+2:]
			}
			fmt.Println("   ", line)
		}
	}
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
