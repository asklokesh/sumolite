package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

func runDoctor() {
	check("gst-launch-1.0", "--version")
	switch runtime.GOOS {
	case "darwin":
		check("gst-inspect-1.0", "vtenc_h264_hw")
		check("gst-inspect-1.0", "avfvideosrc")
	case "linux":
		check("gst-inspect-1.0", "pipewiresrc")
		// try a few hw encoders; at least one should succeed
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
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("  FAIL %s %v: %v\n%s\n", name, args, err, out)
		return
	}
	fmt.Printf("  OK   %s %v\n", name, args)
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
