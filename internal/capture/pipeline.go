package capture

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// FrameSource is the minimal interface a session needs from a capture
// pipeline. Concrete implementations: gstreamer-backed (production) and
// in-memory (tests).
type FrameSource interface {
	// Frame blocks until the next access unit is available and returns
	// its Annex-B bytes (with start codes intact).
	Frame() ([]byte, error)
	Close() error
}

// Pipeline is a running gst-launch process feeding an Annex-B framer.
type Pipeline struct {
	cmd    *exec.Cmd
	stderr io.ReadCloser
	framer *AnnexBFramer
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
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start gst-launch: %w", err)
	}
	return &Pipeline{
		cmd:    cmd,
		stderr: stderr,
		framer: NewAnnexBFramer(out),
	}, nil
}

func (p *Pipeline) Frame() ([]byte, error) { return p.framer.Next() }

func (p *Pipeline) Close() error {
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
	}
	return nil
}

// splitArgs is a tiny shell-like splitter that respects double-quoted
// strings but not escapes. Good enough for our generated pipelines.
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
