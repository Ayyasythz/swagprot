// Package swagprot serves an interactive, Swagger-style web UI for gRPC APIs.
//
// It introspects a target API — via the gRPC Server Reflection API, a set of
// .proto files, or a precompiled FileDescriptorSet — and exposes an http.Handler
// that lets users browse services and methods and invoke RPCs ("try it out")
// straight from the browser, including streaming methods. No generated stubs
// for the target API are required.
//
// The handler can be served standalone or embedded under a sub-path in an
// existing HTTP router (set Config.BasePath). For frameworks built on fasthttp
// (e.g. Fiber) wrap the handler with that framework's net/http adaptor; see the
// examples directory.
//
// Typical standalone usage:
//
//	srv, err := swagprot.NewServer(ctx, swagprot.Config{
//		Dial: swagprot.DialOptions{Address: "localhost:50051", Plaintext: true},
//	})
//	if err != nil { log.Fatal(err) }
//	defer srv.Close()
//	http.ListenAndServe(":8080", srv)
package swagprot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bbyasyi/swagprot/internal/descsource"
	"github.com/bbyasyi/swagprot/internal/invoke"
	"github.com/bbyasyi/swagprot/internal/web"
	"google.golang.org/grpc"
)

// DialOptions controls how swagprot connects to the target gRPC server for
// reflection and invocation. It is ignored when Config.Conn is set.
type DialOptions struct {
	Address   string // host:port
	Plaintext bool   // use plaintext h2c (no TLS)

	// TLS settings (ignored when Plaintext is true).
	CACertFile     string // PEM CA bundle for verifying the server
	ClientCertFile string // client certificate for mTLS
	ClientKeyFile  string // client key for mTLS
	ServerName     string // override the TLS server name / :authority
	Insecure       bool   // skip server certificate verification
}

// Config selects the descriptor source, the invocation target, and how the
// handler is mounted.
//
// Descriptor source resolution (first match wins):
//   - DescriptorSet set        → load that FileDescriptorSet
//   - ProtoFiles non-empty     → compile those .proto files
//   - otherwise                → gRPC reflection over the connection
//
// The connection used for reflection and invocation is Config.Conn if set,
// otherwise one dialed from Config.Dial. With a static source and no
// connection, the UI is browse-only.
type Config struct {
	// Dial configures a connection swagprot opens itself. Ignored if Conn is set.
	Dial DialOptions

	// Conn is an existing gRPC connection to reuse (e.g. the one your service
	// already holds, with its credentials and interceptors). When set, swagprot
	// does not dial or close it — the caller owns its lifecycle.
	Conn *grpc.ClientConn

	// Static descriptor sources (optional; override reflection for browsing).
	ProtoFiles    []string
	ImportPaths   []string
	DescriptorSet string

	// BasePath mounts the UI and API under a sub-path such as "/swagger" so the
	// handler can be embedded in another router. Leave empty to serve at root.
	BasePath string

	// DefaultMetadata pre-populates the UI's request metadata box, as
	// "Key: value" lines.
	DefaultMetadata []string
}

// Server is an http.Handler serving the swagprot UI and API. Close releases any
// resources swagprot owns (a connection it dialed, and the descriptor source).
type Server struct {
	http.Handler
	closers []io.Closer
}

// Close releases resources owned by the server.
func (s *Server) Close() error {
	var first error
	for _, c := range s.closers {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// NewServer builds a Server from cfg.
func NewServer(ctx context.Context, cfg Config) (*Server, error) {
	var closers []io.Closer

	// Resolve the connection: reuse the caller's, or dial our own.
	conn := cfg.Conn
	if conn == nil && cfg.Dial.Address != "" {
		c, err := invoke.Dial(toInvokeDial(cfg.Dial))
		if err != nil {
			return nil, err
		}
		conn = c
		closers = append(closers, conn) // only close connections we opened
	}

	source, err := buildSource(ctx, cfg, conn)
	if err != nil {
		closeAll(closers)
		return nil, err
	}
	closers = append(closers, source)

	var handler http.Handler = web.New(source, conn, cfg.DefaultMetadata)
	if cfg.BasePath != "" {
		handler = withBasePath(cfg.BasePath, handler)
	}
	return &Server{Handler: handler, closers: closers}, nil
}

func buildSource(ctx context.Context, cfg Config, conn *grpc.ClientConn) (descsource.Source, error) {
	switch {
	case cfg.DescriptorSet != "":
		return descsource.FromDescriptorSet(cfg.DescriptorSet)
	case len(cfg.ProtoFiles) > 0:
		return descsource.FromProtoFiles(ctx, cfg.ImportPaths, cfg.ProtoFiles)
	case conn != nil:
		return descsource.FromReflection(ctx, conn), nil
	default:
		return nil, fmt.Errorf("no descriptor source configured: set Dial.Address, Conn, ProtoFiles, or DescriptorSet")
	}
}

func toInvokeDial(o DialOptions) invoke.DialOptions {
	return invoke.DialOptions{
		Address:        o.Address,
		Plaintext:      o.Plaintext,
		CACertFile:     o.CACertFile,
		ClientCertFile: o.ClientCertFile,
		ClientKeyFile:  o.ClientKeyFile,
		ServerName:     o.ServerName,
		Insecure:       o.Insecure,
	}
}

// withBasePath serves h under prefix, stripping it before routing and
// redirecting the bare prefix to its trailing-slash form so the UI's relative
// URLs resolve correctly.
func withBasePath(prefix string, h http.Handler) http.Handler {
	prefix = "/" + strings.Trim(prefix, "/")
	stripped := http.StripPrefix(prefix, h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == prefix {
			http.Redirect(w, r, prefix+"/", http.StatusMovedPermanently)
			return
		}
		stripped.ServeHTTP(w, r)
	})
}

func closeAll(closers []io.Closer) {
	for _, c := range closers {
		_ = c.Close()
	}
}
