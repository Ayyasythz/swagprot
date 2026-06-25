package descsource

import (
	"context"
	"fmt"

	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// reflectionSource resolves descriptors from a live server using the gRPC
// Server Reflection API. The supplied connection is owned by the caller and is
// not closed here.
type reflectionSource struct {
	client *grpcreflect.Client
}

// FromReflection builds a Source backed by the gRPC reflection service on conn.
// NewClientAuto negotiates between the v1 and v1alpha reflection protocols.
func FromReflection(ctx context.Context, conn *grpc.ClientConn) Source {
	return &reflectionSource{client: grpcreflect.NewClientAuto(ctx, conn)}
}

func (r *reflectionSource) ListServices(_ context.Context) ([]protoreflect.ServiceDescriptor, error) {
	names, err := r.client.ListServices()
	if err != nil {
		return nil, fmt.Errorf("list services via reflection: %w", err)
	}
	out := make([]protoreflect.ServiceDescriptor, 0, len(names))
	for _, name := range names {
		if isReflectionService(name) {
			continue
		}
		sd, err := r.client.ResolveService(name)
		if err != nil {
			return nil, fmt.Errorf("resolve service %q: %w", name, err)
		}
		out = append(out, sd.UnwrapService())
	}
	return out, nil
}

func (r *reflectionSource) FindMethod(_ context.Context, fullMethod string) (protoreflect.MethodDescriptor, error) {
	service, method, err := splitMethod(fullMethod)
	if err != nil {
		return nil, err
	}
	sd, err := r.client.ResolveService(service)
	if err != nil {
		return nil, fmt.Errorf("resolve service %q: %w", service, err)
	}
	md := sd.UnwrapService().Methods().ByName(protoreflect.Name(method))
	if md == nil {
		return nil, fmt.Errorf("method %q not found in service %q", method, service)
	}
	return md, nil
}

func (r *reflectionSource) Close() error {
	r.client.Reset()
	return nil
}
