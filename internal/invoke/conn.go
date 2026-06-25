// Package invoke dials a target gRPC server and performs dynamic RPCs using
// descriptors only — no generated stubs. Requests and responses are carried as
// dynamicpb messages and converted to/from proto3 JSON via protojson.
package invoke

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// DialOptions controls how the target server is contacted.
type DialOptions struct {
	Address   string // host:port
	Plaintext bool   // use h2c (no TLS)
	// TLS settings (ignored when Plaintext is true).
	CACertFile     string
	ClientCertFile string
	ClientKeyFile  string
	ServerName     string // override the TLS server name / :authority
	Insecure       bool   // skip server certificate verification
}

// Dial establishes a connection to the target server.
func Dial(opts DialOptions) (*grpc.ClientConn, error) {
	if opts.Address == "" {
		return nil, fmt.Errorf("no target address configured")
	}
	var dialOpts []grpc.DialOption

	if opts.Plaintext {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		creds, err := tlsCredentials(opts)
		if err != nil {
			return nil, err
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	}
	if opts.ServerName != "" {
		dialOpts = append(dialOpts, grpc.WithAuthority(opts.ServerName))
	}

	conn, err := grpc.NewClient(opts.Address, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", opts.Address, err)
	}
	return conn, nil
}

func tlsCredentials(opts DialOptions) (credentials.TransportCredentials, error) {
	cfg := &tls.Config{
		ServerName:         opts.ServerName,
		InsecureSkipVerify: opts.Insecure,
	}
	if opts.CACertFile != "" {
		pem, err := os.ReadFile(opts.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no certificates found in %s", opts.CACertFile)
		}
		cfg.RootCAs = pool
	}
	if opts.ClientCertFile != "" || opts.ClientKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(opts.ClientCertFile, opts.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client cert/key: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return credentials.NewTLS(cfg), nil
}
