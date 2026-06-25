package swagprot_test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/bbyasyi/swagprot/examples/greeter/greetpb"
	"github.com/bbyasyi/swagprot/internal/descsource"
	"github.com/bbyasyi/swagprot/internal/invoke"
	"github.com/bbyasyi/swagprot/internal/schema"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type greetServer struct {
	greetpb.UnimplementedGreeterServer
}

func (greetServer) SayHello(_ context.Context, req *greetpb.HelloRequest) (*greetpb.HelloReply, error) {
	return &greetpb.HelloReply{Message: "Hello, " + req.GetName(), SentAt: timestamppb.Now()}, nil
}

func (greetServer) SayHelloStream(req *greetpb.HelloStreamRequest, stream greetpb.Greeter_SayHelloStreamServer) error {
	for i := int32(1); i <= req.GetCount(); i++ {
		if err := stream.Send(&greetpb.HelloReply{Message: fmt.Sprintf("Hi %s %d", req.GetName(), i)}); err != nil {
			return err
		}
	}
	return nil
}

func dialBufconn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	greetpb.RegisterGreeterServer(s, greetServer{})
	reflection.Register(s)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestReflectionListAndUnary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn := dialBufconn(t)

	src := descsource.FromReflection(ctx, conn)
	svcs, err := src.ListServices(ctx)
	if err != nil {
		t.Fatalf("list services: %v", err)
	}
	var found bool
	for _, sd := range svcs {
		if sd.FullName() == "greet.Greeter" {
			found = true
		}
		if strings.Contains(string(sd.FullName()), "ServerReflection") {
			t.Errorf("reflection service should be filtered out, got %s", sd.FullName())
		}
	}
	if !found {
		t.Fatalf("greet.Greeter not found in %v", svcs)
	}

	md, err := src.FindMethod(ctx, "greet.Greeter.SayHello")
	if err != nil {
		t.Fatalf("find method: %v", err)
	}
	res, err := invoke.Unary(ctx, conn, md, `{"name":"Ada"}`, nil)
	if err != nil {
		t.Fatalf("unary: %v", err)
	}
	if res.Status.Code != "OK" {
		t.Fatalf("status = %s: %s", res.Status.Code, res.Status.Message)
	}
	if !strings.Contains(res.Response, "Hello, Ada") {
		t.Fatalf("response = %q, want it to contain %q", res.Response, "Hello, Ada")
	}
}

func TestServerStreaming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn := dialBufconn(t)
	src := descsource.FromReflection(ctx, conn)

	md, err := src.FindMethod(ctx, "greet.Greeter.SayHelloStream")
	if err != nil {
		t.Fatalf("find method: %v", err)
	}
	var got []string
	res, err := invoke.Stream(ctx, conn, md, []string{`{"name":"Ada","count":3}`}, nil, func(body string) error {
		got = append(got, body)
		return nil
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if res.Status.Code != "OK" {
		t.Fatalf("status = %s: %s", res.Status.Code, res.Status.Message)
	}
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3", len(got))
	}
}

func TestSchemaForRequest(t *testing.T) {
	md := greetpb.File_greeter_proto.Services().ByName("Greeter").Methods().ByName("SayHello")
	detail := schema.BuildMethodDetail(md)
	if detail.Method.FullName != "greet.Greeter.SayHello" {
		t.Fatalf("full name = %s", detail.Method.FullName)
	}
	fields := map[string]schema.Field{}
	for _, f := range detail.Request.Fields {
		fields[f.Name] = f
	}
	if fields["language"].Type != "enum" {
		t.Errorf("language type = %s, want enum", fields["language"].Type)
	}
	if len(fields["language"].EnumValues) == 0 {
		t.Errorf("language should have enum values")
	}
	if !fields["titles"].Repeated {
		t.Errorf("titles should be repeated")
	}
}
