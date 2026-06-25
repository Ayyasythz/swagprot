// Package descsource provides a uniform way to obtain gRPC service/method
// descriptors regardless of where they come from: a live server (via the gRPC
// Server Reflection API), a set of .proto source files, or a precompiled
// FileDescriptorSet. Everything is expressed in terms of the
// google.golang.org/protobuf protoreflect descriptor types so the rest of
// swagprot never depends on generated stubs for the target API.
package descsource

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Source yields descriptors for the services exposed by a target gRPC API.
type Source interface {
	// ListServices returns every service descriptor known to the source,
	// excluding the gRPC reflection service itself.
	ListServices(ctx context.Context) ([]protoreflect.ServiceDescriptor, error)
	// FindMethod resolves a fully-qualified method name of the form
	// "pkg.Service.Method" to its descriptor.
	FindMethod(ctx context.Context, fullMethod string) (protoreflect.MethodDescriptor, error)
	// Close releases any resources held by the source (e.g. a reflection
	// connection). It is safe to call on sources that hold nothing.
	Close() error
}

// splitMethod splits "pkg.Service.Method" into ("pkg.Service", "Method").
func splitMethod(fullMethod string) (service, method string, err error) {
	i := strings.LastIndex(fullMethod, ".")
	if i <= 0 || i == len(fullMethod)-1 {
		return "", "", fmt.Errorf("invalid method name %q: want pkg.Service.Method", fullMethod)
	}
	return fullMethod[:i], fullMethod[i+1:], nil
}

// isReflectionService reports whether name is one of the gRPC reflection
// services, which we never want to expose in the UI.
func isReflectionService(name string) bool {
	return name == "grpc.reflection.v1.ServerReflection" ||
		name == "grpc.reflection.v1alpha.ServerReflection"
}
