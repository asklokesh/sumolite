// Package signaling carries SDP offers/answers between the browser client
// and the local session hub over a WebSocket. The pairing token gates the
// upgrade so a random LAN scanner can't open a session.
package signaling

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/asklokesh/sumolite/internal/session"
	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
)

type Handler struct {
	hub   *session.Hub
	token string
}

func NewHandler(hub *session.Hub, token string) http.Handler {
	return &Handler{hub: hub, token: token}
}

type clientMsg struct {
	Type  string                     `json:"type"`
	Token string                     `json:"token,omitempty"`
	SDP   *webrtc.SessionDescription `json:"sdp,omitempty"`
}

type serverMsg struct {
	Type string                     `json:"type"`
	SDP  *webrtc.SessionDescription `json:"sdp,omitempty"`
	Err  string                     `json:"error,omitempty"`
}

var sessionSeq atomic.Uint64

// newSessionID returns a non-colliding session id. Using r.RemoteAddr
// alone is wrong because the same client reconnecting from the same
// ephemeral port would collide.
func newSessionID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:]) + "-" +
		hex.EncodeToString([]byte{byte(sessionSeq.Add(1))})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // LAN deployments rarely have a matching Origin
	})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusInternalError, "bye")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	authed := false
	// 10s deadline on the auth handshake; nothing else should be tolerated
	// from a peer that hasn't proven the pairing token.
	authCtx, authCancel := context.WithTimeout(ctx, 10*time.Second)
	defer authCancel()

	for {
		readCtx := ctx
		if !authed {
			readCtx = authCtx
		}
		var m clientMsg
		if err := readJSON(readCtx, c, &m); err != nil {
			return
		}
		switch m.Type {
		case "auth":
			if subtle.ConstantTimeCompare([]byte(m.Token), []byte(h.token)) != 1 {
				_ = writeJSON(ctx, c, serverMsg{Type: "error", Err: "bad token"})
				return
			}
			authed = true
			_ = writeJSON(ctx, c, serverMsg{Type: "ok"})
		case "offer":
			if !authed {
				_ = writeJSON(ctx, c, serverMsg{Type: "error", Err: "auth first"})
				return
			}
			if m.SDP == nil {
				_ = writeJSON(ctx, c, serverMsg{Type: "error", Err: "missing sdp"})
				return
			}
			s, err := h.hub.Open(newSessionID())
			if err != nil {
				_ = writeJSON(ctx, c, serverMsg{Type: "error", Err: err.Error()})
				return
			}
			ans, err := s.AnswerOffer(*m.SDP)
			if err != nil {
				s.Close()
				_ = writeJSON(ctx, c, serverMsg{Type: "error", Err: err.Error()})
				return
			}
			_ = writeJSON(ctx, c, serverMsg{Type: "answer", SDP: &ans})
		default:
			_ = writeJSON(ctx, c, serverMsg{Type: "error", Err: "unknown type"})
		}
	}
}

func readJSON(ctx context.Context, c *websocket.Conn, v any) error {
	_, data, err := c.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(ctx context.Context, c *websocket.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Write(ctx, websocket.MessageText, data)
}
