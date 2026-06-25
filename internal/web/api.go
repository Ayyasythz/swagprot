package web

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/bbyasyi/swagprot/internal/invoke"
	"github.com/bbyasyi/swagprot/internal/schema"
)

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"canInvoke":       s.conn != nil,
		"defaultMetadata": s.defaultMetadata,
	})
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	svcs, err := s.source.ListServices(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	out := make([]schema.Service, 0, len(svcs))
	for _, sd := range svcs {
		out = append(out, schema.BuildService(sd))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FullName < out[j].FullName })
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleMethod(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "missing 'name' query parameter")
		return
	}
	md, err := s.source.FindMethod(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, schema.BuildMethodDetail(md))
}

type invokeRequest struct {
	Method   string   `json:"method"`
	Request  string   `json:"request"`
	Metadata []string `json:"metadata"`
}

func (s *Server) handleInvoke(w http.ResponseWriter, r *http.Request) {
	if s.conn == nil {
		writeError(w, http.StatusServiceUnavailable, "invocation is disabled: no target address configured")
		return
	}
	var req invokeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	md, err := s.source.FindMethod(r.Context(), req.Method)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if md.IsStreamingClient() || md.IsStreamingServer() {
		writeError(w, http.StatusBadRequest, "method is streaming; use the streaming endpoint")
		return
	}
	res, err := invoke.Unary(r.Context(), s.conn, md, req.Request, invoke.ParseMetadata(req.Metadata))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type invokeStreamRequest struct {
	Method   string   `json:"method"`
	Requests []string `json:"requests"`
	Metadata []string `json:"metadata"`
}

type invokeStreamResponse struct {
	Messages []string          `json:"messages"`
	Status   invoke.StatusInfo `json:"status"`
	Headers  map[string]string `json:"headers,omitempty"`
	Trailers map[string]string `json:"trailers,omitempty"`
}

// handleInvokeStream runs a streaming RPC and buffers every response message
// into a single JSON reply. It is the non-live fallback used by the UI when a
// WebSocket cannot be established (e.g. behind a fasthttp/Fiber adaptor or a
// buffering proxy).
func (s *Server) handleInvokeStream(w http.ResponseWriter, r *http.Request) {
	if s.conn == nil {
		writeError(w, http.StatusServiceUnavailable, "invocation is disabled: no target address configured")
		return
	}
	var req invokeStreamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	md, err := s.source.FindMethod(r.Context(), req.Method)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if !md.IsStreamingClient() && !md.IsStreamingServer() {
		writeError(w, http.StatusBadRequest, "method is not streaming; use /api/invoke")
		return
	}
	msgs := make([]string, 0)
	res, err := invoke.Stream(r.Context(), s.conn, md, req.Requests, invoke.ParseMetadata(req.Metadata),
		func(body string) error {
			msgs = append(msgs, body)
			return nil
		})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, invokeStreamResponse{
		Messages: msgs,
		Status:   res.Status,
		Headers:  res.Headers,
		Trailers: res.Trailers,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
