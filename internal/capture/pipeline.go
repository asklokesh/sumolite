package capture

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
)

// FrameSource is the minimal interface a session needs from a capture
// pipeline. Concrete implementations: subprocess-backed (production) and
// in-memory (tests).
type FrameSource interface {
	Frame() ([]byte, error)
	Close() error
}

// Pipeline is a running capture subprocess (gst-launch or ffmpeg) feeding
// an Annex-B framer. Wraps the subprocess stdout in a byte counter so
// we can log throughput and detect stalls, and watches the subprocess
// for unexpected exit.
type Pipeline struct {
	cmd        *exec.Cmd
	framer     *AnnexBFramer
	bytesRead  *atomic.Uint64
	framesOut  *atomic.Uint64
	stopReport context.CancelFunc
	exited     atomic.Bool
	exitErr    atomic.Pointer[error]
}

type countingReader struct {
	r io.Reader
	n *atomic.Uint64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.n.Add(uint64(n))
	}
	return n, err
}

// Start spawns the capture subprocess. stderr is forwarded to the parent
// so encoder errors are visible. A reporter goroutine emits a one-line
// throughput log each second so a black-screen-but-RTT-fine situation
// can be diagnosed at a glance.
func (b *Backend) Start(ctx context.Context, bitrateKbps int, pref string) (*Pipeline, error) {
	if b.program == "" || b.argsFor == nil {
		return nil, fmt.Errorf("backend not configured")
	}
	args := b.argsFor(bitrateKbps, pref)
	log.Printf("capture: exec %s %v", b.program, args)

	cmd := exec.CommandContext(ctx, b.program, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", b.program, err)
	}
	log.Printf("capture: %s pid=%d started", b.program, cmd.Process.Pid)

	bytesRead := &atomic.Uint64{}
	framesOut := &atomic.Uint64{}
	cr := &countingReader{r: out, n: bytesRead}
	p := &Pipeline{
		cmd:       cmd,
		framer:    NewAnnexBFramer(cr),
		bytesRead: bytesRead,
		framesOut: framesOut,
	}

	// Watch for subprocess exit so we can attribute downstream EOFs to
	// "ffmpeg died" instead of generic read errors.
	go func() {
		err := cmd.Wait()
		p.exited.Store(true)
		if err != nil {
			p.exitErr.Store(&err)
		}
		log.Printf("capture: %s pid=%d exited (err=%v)", b.program, cmd.Process.Pid, err)
	}()

	reportCtx, cancel := context.WithCancel(ctx)
	p.stopReport = cancel
	go p.reporter(reportCtx, b.program)

	return p, nil
}

func (p *Pipeline) Frame() ([]byte, error) {
	b, err := p.framer.Next()
	if err == nil {
		p.framesOut.Add(1)
	}
	return b, err
}

// reporter emits a throughput line each second. If 3 seconds pass with
// zero bytes read, it logs a loud warning with the likely diagnosis.
func (p *Pipeline) reporter(ctx context.Context, prog string) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	var lastBytes, lastFrames uint64
	warned := false
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			b := p.bytesRead.Load()
			f := p.framesOut.Load()
			db := b - lastBytes
			df := f - lastFrames
			lastBytes, lastFrames = b, f
			log.Printf("capture: %s bytes=%d (+%d/s)  frames=%d (+%d/s)", prog, b, db, f, df)
			if !warned && b == 0 && time.Since(start) > 3*time.Second {
				warned = true
				log.Printf("capture: WARNING — %s has emitted 0 bytes after 3s. Likely macOS Screen Recording permission denied for the binary. Add /opt/homebrew/Cellar/ffmpeg/<version>/bin/ffmpeg to System Settings -> Privacy & Security -> Screen & System Audio Recording.", prog)
			}
		}
	}
}

func (p *Pipeline) Close() error {
	if p.stopReport != nil {
		p.stopReport()
	}
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
