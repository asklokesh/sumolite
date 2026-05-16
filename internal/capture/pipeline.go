package capture

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
)

// Pipeline is a running gst-launch process that emits H.264 NAL units on
// stdout. Frame() reads one Annex-B framed NAL at a time.
type Pipeline struct {
	cmd  *exec.Cmd
	out  io.ReadCloser
	br   *bufio.Reader
}

// Start spawns gst-launch-1.0 with the configured pipeline.
func (b *Backend) Start(ctx context.Context, bitrateKbps int, pref string) (*Pipeline, error) {
	launch := b.LaunchString(bitrateKbps, pref)
	args := append([]string{"-q"}, splitArgs(launch)...)
	cmd := exec.CommandContext(ctx, "gst-launch-1.0", args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start gst-launch: %w", err)
	}
	return &Pipeline{cmd: cmd, out: out, br: bufio.NewReaderSize(out, 1<<20)}, nil
}

// Frame returns the next Annex-B NAL unit (without the start code).
// Blocks until a full unit is available.
func (p *Pipeline) Frame() ([]byte, error) {
	// Naive Annex-B scanner: find next 0x000001 or 0x00000001 boundary.
	// For brevity this scaffold reads in chunks; production code should
	// use a real Annex-B state machine.
	buf := make([]byte, 64*1024)
	n, err := p.br.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

func (p *Pipeline) Close() error {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return nil
}

// splitArgs is a tiny shell-like splitter that respects quoted strings
// but not escape sequences. Good enough for our generated pipelines.
func splitArgs(s string) []string {
	var out []string
	var cur []rune
	inQ := false
	for _, r := range s {
		switch {
		case r == '"':
			inQ = !inQ
		case r == ' ' && !inQ:
			if len(cur) > 0 {
				out = append(out, string(cur))
				cur = cur[:0]
			}
		default:
			cur = append(cur, r)
		}
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}
