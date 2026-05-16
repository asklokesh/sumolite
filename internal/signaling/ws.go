// Package signaling carries SDP offers/answers between the browser client
// and the local session hub over a WebSocket. The pairing token gates the
// upgrade so a random LAN scanner can't open a session.
package signaling

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"

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
	Type  string                    `json:"type"`
	Token string                    `json:"token,omitempty"`
	SDP   *webrtc.SessionDescription `json:"sdp,omitempty"`
}

type serverMsg struct {
	Type string                     `json:"type"`
	SDP  *webrtc.SessionDescription `json:"sdp,omitempty"`
	Err  string                     `json:"error,omitempty"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // LAN deployments rarely have matching Origin
	})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusInternalError, "bye")
	ctx := r.Context()

	for {
		var m clientMsg
		if err := readJSON(ctx, c, &m); err != nil {
			return
		}
		switch m.Type {
		case "auth":
			if subtle.ConstantTimeCompare([]byte(m.Token), []byte(h.token)) != 1 {
				_ = writeJSON(ctx, c, serverMsg{Type: "error", Err: "bad token"})
				return
			}
			_ = writeJSON(ctx, c, serverMsg{Type: "ok"})
		case "offer":
			s, err := h.hub.New(r.RemoteAddr)
			if err != nil {
				_ = writeJSON(ctx, c, serverMsg{Type: "error", Err: err.Error()})
				return
			}
			ans, err := s.AnswerOffer(*m.SDP)
			if err != nil {
				_ = writeJSON(ctx, c, serverMsg{Type: "error", Err: err.Error()})
				return
			}
			_ = writeJSON(ctx, c, serverMsg{Type: "answer", SDP: &ans})
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
