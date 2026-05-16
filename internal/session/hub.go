// Package session owns the lifecycle of a single remote-desktop session:
// the capture source, the WebRTC peer connection, the input data channel,
// and the back-pressure feedback loop.
package session

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/asklokesh/sumolite/internal/capture"
	"github.com/asklokesh/sumolite/internal/input"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// SourceFactory builds a fresh FrameSource for a new session. The session
// owns the source and closes it on shutdown.
type SourceFactory func(ctx context.Context) (capture.FrameSource, error)

type Hub struct {
	newSource SourceFactory
	newInput  func() (input.Injector, error)
	frameDur  time.Duration

	mu       sync.Mutex
	sessions map[string]*Session
}

type Options struct {
	NewSource SourceFactory
	NewInput  func() (input.Injector, error)
	FPS       int
}

// New creates a Hub with the supplied factories. In production, the
// caller wires NewSource from capture.Backend.Start. Tests pass a fake.
func New(opt Options) *Hub {
	if opt.NewInput == nil {
		opt.NewInput = input.New
	}
	if opt.FPS <= 0 {
		opt.FPS = 60
	}
	return &Hub{
		newSource: opt.NewSource,
		newInput:  opt.NewInput,
		frameDur:  time.Second / time.Duration(opt.FPS),
		sessions:  map[string]*Session{},
	}
}

// NewFromBackend is the production wiring.
func NewFromBackend(b *capture.Backend, bitrate int, encoder string, fps int) *Hub {
	return New(Options{
		NewSource: func(ctx context.Context) (capture.FrameSource, error) {
			return b.Start(ctx, bitrate, encoder)
		},
		FPS: fps,
	})
}

type Session struct {
	ID     string
	pc     *webrtc.PeerConnection
	vid    *webrtc.TrackLocalStaticSample
	in     input.Injector
	ctx    context.Context
	cancel context.CancelFunc

	closed atomic.Bool

	OnClose func()
}

// Open creates a session and starts the capture pump. It returns the
// session ready for SDP exchange.
func (h *Hub) Open(id string) (*Session, error) {
	api := webrtc.NewAPI()
	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}},
	})
	if err != nil {
		return nil, err
	}
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video", "sumolite",
	)
	if err != nil {
		_ = pc.Close()
		return nil, err
	}
	if _, err := pc.AddTrack(track); err != nil {
		_ = pc.Close()
		return nil, err
	}

	var inj input.Injector
	if h.newInput != nil {
		inj, err = h.newInput()
		if err != nil {
			log.Printf("input injector unavailable: %v (view-only mode)", err)
			inj = nil
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	s := &Session{ID: id, pc: pc, vid: track, in: inj, ctx: ctx, cancel: cancel}

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() == "input" && inj != nil {
			dc.OnMessage(func(msg webrtc.DataChannelMessage) {
				_ = inj.Handle(msg.Data)
			})
		}
	})
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("session %s: %s", id, state)
		switch state {
		case webrtc.PeerConnectionStateFailed,
			webrtc.PeerConnectionStateClosed,
			webrtc.PeerConnectionStateDisconnected:
			s.Close()
		}
	})

	go h.pump(s)

	h.mu.Lock()
	h.sessions[id] = s
	h.mu.Unlock()
	return s, nil
}

func (h *Hub) Get(id string) *Session {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[id]
}

func (h *Hub) remove(id string) {
	h.mu.Lock()
	delete(h.sessions, id)
	h.mu.Unlock()
}

// pump runs the capture pipeline and pushes samples into the WebRTC track.
// Sample durations are derived from wall-clock arrival time of each
// access unit, not a fixed nominal frame interval. Using wall-clock
// duration keeps RTP timestamps aligned with real time even when the
// encoder produces slightly more or fewer frames per second than we
// guessed (e.g. when the user drags a window and the capture rate dips).
func (h *Hub) pump(s *Session) {
	src, err := h.newSource(s.ctx)
	if err != nil {
		log.Printf("capture start: %v", err)
		s.Close()
		return
	}
	defer src.Close()

	var last time.Time
	for {
		if s.closed.Load() {
			return
		}
		buf, err := src.Frame()
		if err != nil {
			if err != io.EOF {
				log.Printf("capture read: %v", err)
			}
			return
		}
		if s.closed.Load() {
			return
		}
		now := time.Now()
		dur := h.frameDur
		if !last.IsZero() {
			dur = now.Sub(last)
		}
		last = now
		if err := s.vid.WriteSample(media.Sample{Data: buf, Duration: dur}); err != nil {
			log.Printf("write sample: %v", err)
			return
		}
	}
}

func (s *Session) AnswerOffer(offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	if err := s.pc.SetRemoteDescription(offer); err != nil {
		return webrtc.SessionDescription{}, err
	}
	answer, err := s.pc.CreateAnswer(nil)
	if err != nil {
		return webrtc.SessionDescription{}, err
	}
	gatherComplete := webrtc.GatheringCompletePromise(s.pc)
	if err := s.pc.SetLocalDescription(answer); err != nil {
		return webrtc.SessionDescription{}, err
	}
	select {
	case <-gatherComplete:
	case <-time.After(5 * time.Second):
		return webrtc.SessionDescription{}, fmt.Errorf("ICE gather timeout")
	}
	return *s.pc.LocalDescription(), nil
}

// Close is idempotent. Multiple paths can race to close a session
// (signaling drop, peer-connection state change, ctx cancel) so we guard
// with an atomic and let the first caller do the real work.
func (s *Session) Close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}
	s.cancel()
	_ = s.pc.Close()
	if s.in != nil {
		_ = s.in.Close()
	}
	if s.OnClose != nil {
		s.OnClose()
	}
}
