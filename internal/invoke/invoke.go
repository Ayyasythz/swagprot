package invoke

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// StatusInfo is the JSON-friendly view of a gRPC status returned to the UI.
type StatusInfo struct {
	Code    string `json:"code"`    // e.g. "OK", "NotFound"
	Number  uint32 `json:"number"`  // numeric status code
	Message string `json:"message"` // status message, if any
}

// Result is the outcome of a unary call.
type Result struct {
	Response string            `json:"response,omitempty"` // proto3-JSON of the response message
	Status   StatusInfo        `json:"status"`
	Headers  map[string]string `json:"headers,omitempty"`
	Trailers map[string]string `json:"trailers,omitempty"`
}

// FullMethod returns the gRPC wire path "/pkg.Service/Method" for a method.
func FullMethod(md protoreflect.MethodDescriptor) string {
	svc := md.Parent().(protoreflect.ServiceDescriptor)
	return "/" + string(svc.FullName()) + "/" + string(md.Name())
}

// BuildMessage parses proto3-JSON into a dynamic message of the given type. An
// empty or whitespace-only body yields an empty message.
func BuildMessage(desc protoreflect.MessageDescriptor, jsonBody string) (*dynamicpb.Message, error) {
	msg := dynamicpb.NewMessage(desc)
	if strings.TrimSpace(jsonBody) == "" {
		return msg, nil
	}
	opts := protojson.UnmarshalOptions{DiscardUnknown: false}
	if err := opts.Unmarshal([]byte(jsonBody), msg); err != nil {
		return nil, fmt.Errorf("invalid request JSON: %w", err)
	}
	return msg, nil
}

// MarshalMessage renders a dynamic message as proto3-JSON.
func MarshalMessage(msg *dynamicpb.Message) (string, error) {
	opts := protojson.MarshalOptions{EmitUnpopulated: true, Indent: "  "}
	b, err := opts.Marshal(msg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// StatusFromError converts a (possibly nil) error into StatusInfo.
func StatusFromError(err error) StatusInfo {
	st := status.Convert(err)
	return StatusInfo{
		Code:    codeName(st.Code()),
		Number:  uint32(st.Code()),
		Message: st.Message(),
	}
}

// Unary performs a unary RPC and returns the response plus status/metadata.
func Unary(ctx context.Context, conn *grpc.ClientConn, md protoreflect.MethodDescriptor, reqJSON string, header metadata.MD) (*Result, error) {
	if md.IsStreamingClient() || md.IsStreamingServer() {
		return nil, fmt.Errorf("method %s is streaming; use the streaming endpoint", md.FullName())
	}
	req, err := BuildMessage(md.Input(), reqJSON)
	if err != nil {
		return nil, err
	}
	resp := dynamicpb.NewMessage(md.Output())

	if len(header) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, header)
	}
	var respHeader, respTrailer metadata.MD
	callErr := conn.Invoke(ctx, FullMethod(md), req, resp,
		grpc.Header(&respHeader), grpc.Trailer(&respTrailer))

	res := &Result{
		Status:   StatusFromError(callErr),
		Headers:  flatten(respHeader),
		Trailers: flatten(respTrailer),
	}
	if callErr == nil {
		body, err := MarshalMessage(resp)
		if err != nil {
			return nil, fmt.Errorf("marshal response: %w", err)
		}
		res.Response = body
	}
	return res, nil
}

func flatten(md metadata.MD) map[string]string {
	if len(md) == 0 {
		return nil
	}
	out := make(map[string]string, len(md))
	for k, v := range md {
		out[k] = strings.Join(v, ", ")
	}
	return out
}

// ParseMetadata turns "Key: value" header lines into gRPC metadata.
func ParseMetadata(lines []string) metadata.MD {
	md := metadata.MD{}
	for _, line := range lines {
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		if key == "" {
			continue
		}
		md.Append(strings.ToLower(key), val)
	}
	return md
}

func codeName(c codes.Code) string {
	if s := c.String(); s != "" {
		return s
	}
	return fmt.Sprintf("Code(%d)", c)
}
