// Command swagprot serves an interactive Swagger-style web UI for a gRPC API.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/Ayyasythz/swagprot"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		addr        string
		protoFiles  []string
		importPaths []string
		descriptor  string

		plaintext  bool
		insecure   bool
		caCert     string
		clientCert string
		clientKey  string
		authority  string

		headers  []string
		port     int
		basePath string
		open     bool
	)

	cmd := &cobra.Command{
		Use:   "swagprot",
		Short: "Interactive web UI for gRPC APIs — Swagger for gRPC",
		Long: `swagprot introspects a gRPC API and serves a browser UI to explore
services and methods and invoke RPCs ("try it out"), including streaming.

Descriptors come from gRPC server reflection (default, via --addr), from
.proto files (--proto), or from a compiled FileDescriptorSet (--descriptor).
Invocation requires --addr.`,
		Example: `  # Reflection-based, plaintext server:
  swagprot --addr localhost:50051 --plaintext

  # Browse from .proto files, invoke against a live server:
  swagprot --proto api/greeter.proto --import-path api --addr localhost:50051 --plaintext

  # From a precompiled descriptor set:
  swagprot --descriptor api.binpb --addr localhost:50051 --plaintext`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if addr == "" && len(protoFiles) == 0 && descriptor == "" {
				return fmt.Errorf("need a descriptor source: set --addr, --proto, or --descriptor")
			}
			cfg := swagprot.Config{
				Dial: swagprot.DialOptions{
					Address:        addr,
					Plaintext:      plaintext,
					Insecure:       insecure,
					CACertFile:     caCert,
					ClientCertFile: clientCert,
					ClientKeyFile:  clientKey,
					ServerName:     authority,
				},
				ProtoFiles:      protoFiles,
				ImportPaths:     importPaths,
				DescriptorSet:   descriptor,
				BasePath:        basePath,
				DefaultMetadata: headers,
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			srv, err := swagprot.NewServer(ctx, cfg)
			if err != nil {
				return err
			}
			defer srv.Close()

			listenAddr := fmt.Sprintf(":%d", port)
			ln, err := net.Listen("tcp", listenAddr)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", listenAddr, err)
			}
			url := fmt.Sprintf("http://localhost:%d%s", port, basePath)
			log.Printf("swagprot serving %s", url)
			if addr == "" {
				log.Printf("note: no --addr set; UI is browse-only (invocation disabled)")
			}
			if open {
				go openBrowser(url)
			}
			return http.Serve(ln, srv)
		},
	}

	f := cmd.Flags()
	f.StringVar(&addr, "addr", "", "target gRPC server host:port (reflection + invocation)")
	f.StringSliceVar(&protoFiles, "proto", nil, "path(s) to .proto files to load instead of reflection")
	f.StringSliceVar(&importPaths, "import-path", nil, "import path(s) for resolving .proto imports")
	f.StringVar(&descriptor, "descriptor", "", "path to a binary FileDescriptorSet")

	f.BoolVar(&plaintext, "plaintext", false, "use plaintext (no TLS)")
	f.BoolVar(&insecure, "insecure", false, "skip TLS certificate verification")
	f.StringVar(&caCert, "cacert", "", "CA certificate file for TLS")
	f.StringVar(&clientCert, "cert", "", "client certificate file for mTLS")
	f.StringVar(&clientKey, "key", "", "client key file for mTLS")
	f.StringVar(&authority, "authority", "", "override the :authority / TLS server name")

	f.StringSliceVarP(&headers, "header", "H", nil, "default request metadata as 'Key: value' (repeatable)")
	f.IntVar(&port, "port", 8080, "port for the web UI")
	f.StringVar(&basePath, "base-path", "", "serve the UI under a sub-path (e.g. /swagger)")
	f.BoolVar(&open, "open", false, "open the UI in a browser on start")

	return cmd
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	_ = exec.Command(cmd, append(args, url)...).Start()
}
