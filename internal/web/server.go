// Package web exposes the swagprot HTTP API and the embedded browser UI.
package web

import (
	"io/fs"
	"net/http"

	"github.com/Ayyasythz/swagprot/internal/descsource"
	"google.golang.org/grpc"
)

// Server implements the swagprot HTTP handler.
type Server struct {
	source          descsource.Source
	conn            *grpc.ClientConn // nil when invocation is disabled
	defaultMetadata []string
	mux             *http.ServeMux
}

// New builds a Server. conn may be nil, in which case the UI is browse-only.
func New(source descsource.Source, conn *grpc.ClientConn, defaultMetadata []string) *Server {
	s := &Server{
		source:          source,
		conn:            conn,
		defaultMetadata: defaultMetadata,
		mux:             http.NewServeMux(),
	}

	s.mux.HandleFunc("GET /api/config", s.handleConfig)
	s.mux.HandleFunc("GET /api/services", s.handleServices)
	s.mux.HandleFunc("GET /api/method", s.handleMethod)
	s.mux.HandleFunc("POST /api/invoke", s.handleInvoke)
	s.mux.HandleFunc("POST /api/invoke-stream", s.handleInvokeStream)
	s.mux.HandleFunc("GET /api/stream", s.handleStream)

	// Static UI assets, served from the embedded filesystem at the root.
	sub, _ := fs.Sub(assetsFS, "assets")
	s.mux.Handle("GET /", http.FileServer(http.FS(sub)))

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
