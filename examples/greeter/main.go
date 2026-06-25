// Command greeter is a tiny gRPC server used to try swagprot end to end.
// It registers the gRPC reflection service so swagprot can introspect it with
// just `swagprot --addr localhost:50051 --plaintext`.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/bbyasyi/swagprot/examples/greeter/greetpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type server struct {
	greetpb.UnimplementedGreeterServer
}

func greeting(lang greetpb.Language, name string) string {
	if name == "" {
		name = "world"
	}
	switch lang {
	case greetpb.Language_INDONESIAN:
		return "Halo, " + name
	case greetpb.Language_JAPANESE:
		return "こんにちは, " + name
	default:
		return "Hello, " + name
	}
}

func (s *server) SayHello(_ context.Context, req *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	return &greetpb.HelloReply{
		Message: greeting(req.GetLanguage(), req.GetName()),
		SentAt:  timestamppb.Now(),
	}, nil
}

func (s *server) SayHelloStream(req *greetpb.HelloStreamRequest, stream greetpb.Greeter_SayHelloStreamServer) error {
	n := req.GetCount()
	if n <= 0 {
		n = 3
	}
	for i := int32(1); i <= n; i++ {
		if err := stream.Send(&greetpb.HelloReply{
			Message: fmt.Sprintf("%s (%d/%d)", greeting(greetpb.Language_ENGLISH, req.GetName()), i, n),
			SentAt:  timestamppb.Now(),
		}); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

func main() {
	addr := flag.String("addr", ":50051", "listen address")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	s := grpc.NewServer()
	greetpb.RegisterGreeterServer(s, &server{})
	reflection.Register(s)

	log.Printf("greeter listening on %s", ln.Addr())
	if err := s.Serve(ln); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
