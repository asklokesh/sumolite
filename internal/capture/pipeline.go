package capture

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// FrameSource is the minimal interface a session needs from a capture
// pipeline. Concrete implementations: subprocess-backed (production) and
// in-memory (tests).
type FrameSource interface {
	Frame() ([]byte, error)
	Close() error
}

// Pipeline is a running capture subprocess (gst-launch or ffmpeg) feeding
// an Annex-B framer.
type Pipeline struct {
	cmd    *exec.Cmd
	framer *AnnexBFramer
}

// Start spawns the capture subprocess. stderr is forwarded to the
// parent's stderr so encoder errors aren't silently swallowed (this used
// to be a piped reader that no one drained, which deadlocked the
// subprocess as soon as the kernel pipe buffer filled).
func (b *Backend) Start(ctx context.Context, bitrateKbps int, pref string) (*Pipeline, error) {
	if b.program == "" || b.argsFor == nil {
		return nil, fmt.Errorf("backend not configured")
	}
	args := b.argsFor(bitrateKbps, pref)
	cmd := exec.CommandContext(ctx, b.program, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", b.program, err)
	}
	return &Pipeline{cmd: cmd, framer: NewAnnexBFramer(out)}, nil
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

// io.Reader compatibility kept for test rigs that wire a custom source.
var _ io.Reader = (*os.File)(nil)
