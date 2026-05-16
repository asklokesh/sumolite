package session

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/asklokesh/sumolite/internal/capture"
	"github.com/asklokesh/sumolite/internal/input"

	"github.com/pion/webrtc/v4"
)

// captureInjector records events for assertions. Implements input.Injector.
type captureInjector struct {
	got chan []byte
}

func (c *captureInjector) Handle(b []byte) error {
	cp := make([]byte, len(b))
	copy(cp, b)
	select {
	case c.got <- cp:
	default:
	}
	return nil
}
func (c *captureInjector) Close() error { return nil }

// TestHub_VideoFlowsAndInputDelivers wires a real Pion peer connection
// to the hub's session over an in-process SDP exchange, feeds it from
// the fake capture backend, and verifies:
//
//  1. The remote peer receives RTP packets on the video track.
//  2. Mouse events sent over the data channel reach the injector.
func TestHub_VideoFlowsAndInputDelivers(t *testing.T) {
	if testing.Short() {
		t.Skip("network/WebRTC test")
	}

	fake := &capture.FakeBackend{FPS: 120}
	inj := &captureInjector{got: make(chan []byte, 16)}

	hub := New(Options{
		NewSource: func(ctx context.Context) (capture.FrameSource, error) {
			return fake.Start(ctx), nil
		},
		NewInput: func() (input.Injector, error) { return inj, nil },
		FPS:      120,
	})

	server, err := hub.Open("t1")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	// Build the client side.
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if _, err := client.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo,
		webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly}); err != nil {
		t.Fatal(err)
	}

	ordered := false
	zero := uint16(0)
	dc, err := client.CreateDataChannel("input", &webrtc.DataChannelInit{Ordered: &ordered, MaxRetransmits: &zero})
	if err != nil {
		t.Fatal(err)
	}

	var rtpCount atomic.Int32
	client.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		go func() {
			for {
				if _, _, err := track.ReadRTP(); err != nil {
					return
				}
				rtpCount.Add(1)
			}
		}()
	})

	dcOpen := make(chan struct{})
	dc.OnOpen(func() { close(dcOpen) })

	offer, err := client.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gather := webrtc.GatheringCompletePromise(client)
	if err := client.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	select {
	case <-gather:
	case <-time.After(3 * time.Second):
		t.Fatal("client ICE gather timeout")
	}

	answer, err := server.AnswerOffer(*client.LocalDescription())
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SetRemoteDescription(answer); err != nil {
		t.Fatal(err)
	}

	// Wait for the connection.
	connected := make(chan struct{})
	client.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		if s == webrtc.PeerConnectionStateConnected {
			select {
			case <-connected:
			default:
				close(connected)
			}
		}
	})
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("not connected")
	}

	select {
	case <-dcOpen:
	case <-time.After(3 * time.Second):
		t.Fatal("data channel never opened")
	}

	// Send a few input events. The browser sends a fresh ArrayBuffer slice
	// for each event; we mirror that here.
	move := []byte{input.EvMouseMove, 0x10, 0x00, 0x20, 0x00}
	for i := 0; i < 3; i++ {
		if err := dc.Send(move); err != nil {
			t.Fatal(err)
		}
	}

	// Expect at least one event to reach the injector.
	select {
	case got := <-inj.got:
		if got[0] != input.EvMouseMove {
			t.Fatalf("wrong event kind %v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("injector never saw any event")
	}

	// And RTP should be flowing.
	deadline := time.Now().Add(3 * time.Second)
	for rtpCount.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if rtpCount.Load() < 3 {
		t.Fatalf("expected RTP packets, got %d", rtpCount.Load())
	}
}

func TestSessionCloseIdempotent(t *testing.T) {
	fake := &capture.FakeBackend{FPS: 30}
	hub := New(Options{
		NewSource: func(ctx context.Context) (capture.FrameSource, error) {
			return fake.Start(ctx), nil
		},
		NewInput: func() (input.Injector, error) { return &captureInjector{got: make(chan []byte, 1)}, nil },
		FPS:      30,
	})
	s, err := hub.Open("t-close")
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int32
	s.OnClose = func() { n.Add(1) }
	s.Close()
	s.Close()
	s.Close()
	if got := n.Load(); got != 1 {
		t.Fatalf("OnClose fired %d times, want 1", got)
	}
}
