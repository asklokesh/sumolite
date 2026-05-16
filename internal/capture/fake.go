package capture

import (
	"context"
	"io"
	"sync"
	"time"
)

// FakeBackend produces canned H.264 frames at a fixed rate. Used by hub/
// session tests so we don't shell out to gst-launch in CI.
type FakeBackend struct {
	FPS   int
	Frame []byte
}

func (f *FakeBackend) Start(ctx context.Context) FrameSource {
	if f.FPS <= 0 {
		f.FPS = 60
	}
	if f.Frame == nil {
		f.Frame = []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x88, 0x84, 0x00, 0x33}
	}
	return &fakeSource{ctx: ctx, fps: f.FPS, frame: f.Frame, t: time.NewTicker(time.Second / time.Duration(f.FPS))}
}

type fakeSource struct {
	ctx    context.Context
	fps    int
	frame  []byte
	t      *time.Ticker
	mu     sync.Mutex
	closed bool
}

func (s *fakeSource) Frame() ([]byte, error) {
	select {
	case <-s.ctx.Done():
		return nil, io.EOF
	case <-s.t.C:
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.closed {
			return nil, io.EOF
		}
		out := make([]byte, len(s.frame))
		copy(out, s.frame)
		return out, nil
	}
}

func (s *fakeSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.t.Stop()
	}
	return nil
}
