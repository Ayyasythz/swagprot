package web

import (
	"net/http"

	"github.com/bbyasyi/swagprot/internal/invoke"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// streamStart is the first frame the client sends to begin a streaming call.
type streamStart struct {
	Method   string   `json:"method"`
	Requests []string `json:"requests"` // one per client message (proto3-JSON)
	Metadata []string `json:"metadata"`
}

// streamEvent is a frame sent from server to client over the WebSocket.
type streamEvent struct {
	Type     string             `json:"type"` // "message" | "status" | "error"
	Data     string             `json:"data,omitempty"`
	Status   *invoke.StatusInfo `json:"status,omitempty"`
	Headers  map[string]string  `json:"headers,omitempty"`
	Trailers map[string]string  `json:"trailers,omitempty"`
	Error    string             `json:"error,omitempty"`
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	// InsecureSkipVerify keeps the local dev tool usable across the various
	// localhost origins a user might open it from.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	ctx := r.Context()
	defer c.Close(websocket.StatusNormalClosure, "")

	if s.conn == nil {
		_ = wsjson.Write(ctx, c, streamEvent{Type: "error", Error: "invocation is disabled: no target address configured"})
		return
	}

	var start streamStart
	if err := wsjson.Read(ctx, c, &start); err != nil {
		return
	}
	md, err := s.source.FindMethod(ctx, start.Method)
	if err != nil {
		_ = wsjson.Write(ctx, c, streamEvent{Type: "error", Error: err.Error()})
		return
	}
	if !md.IsStreamingClient() && !md.IsStreamingServer() {
		_ = wsjson.Write(ctx, c, streamEvent{Type: "error", Error: "method is not streaming; use the unary endpoint"})
		return
	}

	result, err := invoke.Stream(ctx, s.conn, md, start.Requests, invoke.ParseMetadata(start.Metadata),
		func(body string) error {
			return wsjson.Write(ctx, c, streamEvent{Type: "message", Data: body})
		})
	if err != nil {
		_ = wsjson.Write(ctx, c, streamEvent{Type: "error", Error: err.Error()})
		return
	}
	_ = wsjson.Write(ctx, c, streamEvent{
		Type:     "status",
		Status:   &result.Status,
		Headers:  result.Headers,
		Trailers: result.Trailers,
	})
}
