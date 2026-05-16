// Package session owns the lifecycle of a single remote-desktop session:
// the GStreamer capture pipeline, the WebRTC peer connection, the input
// data channel, and the back-pressure feedback loop.
package session

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/asklokesh/sumolite/internal/capture"
	"github.com/asklokesh/sumolite/internal/input"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

type Hub struct {
	cap     *capture.Backend
	bitrate int
	encoder string

	mu       sync.Mutex
	sessions map[string]*Session
}

func NewHub(cap *capture.Backend, bitrate int, encoder string) *Hub {
	return &Hub{cap: cap, bitrate: bitrate, encoder: encoder, sessions: map[string]*Session{}}
}

type Session struct {
	ID  string
	pc  *webrtc.PeerConnection
	vid *webrtc.TrackLocalStaticSample
	in  input.Injector
	ctx context.Context
	cancel context.CancelFunc
}

// New creates a session and returns the local SDP offer answerer.
// The caller drives signaling: feed remote SDP via Session.AnswerOffer.
func (h *Hub) New(id string) (*Session, error) {
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
		return nil, err
	}
	if _, err := pc.AddTrack(track); err != nil {
		return nil, err
	}

	inj, err := input.New()
	if err != nil {
		log.Printf("input injector unavailable: %v (view-only mode)", err)
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
		if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
			s.Close()
		}
	})

	go h.pump(s)

	h.mu.Lock()
	h.sessions[id] = s
	h.mu.Unlock()
	return s, nil
}

// pump runs the capture pipeline and pushes samples into the WebRTC track.
func (h *Hub) pump(s *Session) {
	pipe, err := h.cap.Start(s.ctx, h.bitrate, h.encoder)
	if err != nil {
		log.Printf("capture start: %v", err)
		return
	}
	defer pipe.Close()

	// 60fps assumption — Pion uses sample duration as RTP timestamp delta.
	frameDur := time.Second / 60

	for {
		if s.ctx.Err() != nil {
			return
		}
		buf, err := pipe.Frame()
		if err != nil {
			log.Printf("capture read: %v", err)
			return
		}
		if err := s.vid.WriteSample(media.Sample{Data: buf, Duration: frameDur}); err != nil {
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

func (s *Session) Close() {
	s.cancel()
	_ = s.pc.Close()
}
