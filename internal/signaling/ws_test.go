package signaling

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asklokesh/sumolite/internal/capture"
	"github.com/asklokesh/sumolite/internal/input"
	"github.com/asklokesh/sumolite/internal/session"
	"nhooyr.io/websocket"
)

func newHub(t *testing.T) *session.Hub {
	t.Helper()
	fake := &capture.FakeBackend{FPS: 30}
	return session.New(session.Options{
		NewSource: func(ctx context.Context) (capture.FrameSource, error) {
			return fake.Start(ctx), nil
		},
		NewInput: func() (input.Injector, error) {
			return nil, &noInjector{}
		},
		FPS: 30,
	})
}

type noInjector struct{}

func (n *noInjector) Error() string { return "no injector in tests" }

func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

func write(t *testing.T, c *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readResp(t *testing.T, c *websocket.Conn) serverMsg {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m serverMsg
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode: %v: %s", err, b)
	}
	return m
}

func TestSignaling_BadTokenRejected(t *testing.T) {
	srv := httptest.NewServer(NewHandler(newHub(t), "secret"))
	defer srv.Close()

	c := dial(t, srv)
	defer c.Close(websocket.StatusNormalClosure, "")

	write(t, c, clientMsg{Type: "auth", Token: "wrong"})
	m := readResp(t, c)
	if m.Type != "error" {
		t.Fatalf("expected error, got %+v", m)
	}
}

func TestSignaling_AuthAcceptsCorrectToken(t *testing.T) {
	srv := httptest.NewServer(NewHandler(newHub(t), "secret"))
	defer srv.Close()

	c := dial(t, srv)
	defer c.Close(websocket.StatusNormalClosure, "")

	write(t, c, clientMsg{Type: "auth", Token: "secret"})
	m := readResp(t, c)
	if m.Type != "ok" {
		t.Fatalf("expected ok, got %+v", m)
	}
}

func TestSignaling_OfferBeforeAuthRejected(t *testing.T) {
	srv := httptest.NewServer(NewHandler(newHub(t), "secret"))
	defer srv.Close()

	c := dial(t, srv)
	defer c.Close(websocket.StatusNormalClosure, "")

	write(t, c, clientMsg{Type: "offer"})
	m := readResp(t, c)
	if m.Type != "error" || m.Err != "auth first" {
		t.Fatalf("expected auth-first error, got %+v", m)
	}
}

func TestSignaling_UnknownTypeRejected(t *testing.T) {
	srv := httptest.NewServer(NewHandler(newHub(t), "secret"))
	defer srv.Close()

	c := dial(t, srv)
	defer c.Close(websocket.StatusNormalClosure, "")

	write(t, c, clientMsg{Type: "auth", Token: "secret"})
	_ = readResp(t, c)

	write(t, c, clientMsg{Type: "ping"})
	m := readResp(t, c)
	if m.Type != "error" {
		t.Fatalf("expected error, got %+v", m)
	}
}

func TestNewSessionID_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := newSessionID()
		if seen[id] {
			t.Fatal("duplicate")
		}
		seen[id] = true
	}
}
