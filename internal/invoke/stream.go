package invoke

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// StreamResult is the terminal outcome of a streaming RPC.
type StreamResult struct {
	Status   StatusInfo        `json:"status"`
	Headers  map[string]string `json:"headers,omitempty"`
	Trailers map[string]string `json:"trailers,omitempty"`
}

// Stream runs a streaming RPC using a non-interactive "send all, then receive"
// model: every request in reqs (proto3-JSON) is sent, the stream is
// half-closed, and onMessage is called with each response message as
// proto3-JSON. This covers server-, client-, and bidi-streaming methods; for
// server-streaming, reqs must contain exactly one request.
func Stream(
	ctx context.Context,
	conn *grpc.ClientConn,
	md protoreflect.MethodDescriptor,
	reqs []string,
	header metadata.MD,
	onMessage func(string) error,
) (*StreamResult, error) {
	if !md.IsStreamingClient() && !md.IsStreamingServer() {
		return nil, fmt.Errorf("method %s is not streaming", md.FullName())
	}
	if !md.IsStreamingClient() && len(reqs) != 1 {
		return nil, fmt.Errorf("server-streaming method %s expects exactly one request", md.FullName())
	}

	if len(header) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, header)
	}
	desc := &grpc.StreamDesc{
		StreamName:    string(md.Name()),
		ServerStreams: md.IsStreamingServer(),
		ClientStreams: md.IsStreamingClient(),
	}
	cs, err := conn.NewStream(ctx, desc, FullMethod(md))
	if err != nil {
		return &StreamResult{Status: StatusFromError(err)}, nil
	}

	// Send phase. A send error means the server aborted early; the real status
	// surfaces from RecvMsg below, so we stop sending and fall through.
	for _, r := range reqs {
		msg, err := BuildMessage(md.Input(), r)
		if err != nil {
			return nil, err
		}
		if err := cs.SendMsg(msg); err != nil {
			break
		}
	}
	_ = cs.CloseSend()

	// Receive phase.
	for {
		out := dynamicpb.NewMessage(md.Output())
		recvErr := cs.RecvMsg(out)
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			return &StreamResult{
				Status:   StatusFromError(recvErr),
				Headers:  headerOf(cs),
				Trailers: flatten(cs.Trailer()),
			}, nil
		}
		body, err := MarshalMessage(out)
		if err != nil {
			return nil, fmt.Errorf("marshal response: %w", err)
		}
		if err := onMessage(body); err != nil {
			return nil, err
		}
	}

	return &StreamResult{
		Status:   StatusFromError(nil),
		Headers:  headerOf(cs),
		Trailers: flatten(cs.Trailer()),
	}, nil
}

func headerOf(cs grpc.ClientStream) map[string]string {
	md, err := cs.Header()
	if err != nil {
		return nil
	}
	return flatten(md)
}
