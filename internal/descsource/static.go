package descsource

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// staticSource serves descriptors out of an in-memory file registry. It backs
// both the .proto-file and FileDescriptorSet sources.
type staticSource struct {
	files *protoregistry.Files
}

func (s *staticSource) ListServices(_ context.Context) ([]protoreflect.ServiceDescriptor, error) {
	var out []protoreflect.ServiceDescriptor
	s.files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		svcs := fd.Services()
		for i := 0; i < svcs.Len(); i++ {
			sd := svcs.Get(i)
			if !isReflectionService(string(sd.FullName())) {
				out = append(out, sd)
			}
		}
		return true
	})
	return out, nil
}

func (s *staticSource) FindMethod(_ context.Context, fullMethod string) (protoreflect.MethodDescriptor, error) {
	d, err := s.files.FindDescriptorByName(protoreflect.FullName(fullMethod))
	if err != nil {
		return nil, fmt.Errorf("resolve method %q: %w", fullMethod, err)
	}
	md, ok := d.(protoreflect.MethodDescriptor)
	if !ok {
		return nil, fmt.Errorf("%q is not a method", fullMethod)
	}
	return md, nil
}

func (s *staticSource) Close() error { return nil }

// newStaticFromSet builds a static source from a FileDescriptorSet. The set
// must be topologically ordered (dependencies before dependents), which is the
// case for sets produced by `protoc --include_imports -o`.
func newStaticFromSet(set *descriptorpb.FileDescriptorSet) (Source, error) {
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, fmt.Errorf("build file registry: %w", err)
	}
	return &staticSource{files: files}, nil
}

// registryFromRoots collects the transitive closure of the given root files in
// dependency order and builds a *protoregistry.Files. Used by the .proto source
// where the compiler hands back already-linked FileDescriptors.
func registryFromRoots(roots []protoreflect.FileDescriptor) (*protoregistry.Files, error) {
	set := &descriptorpb.FileDescriptorSet{}
	seen := map[string]bool{}
	var add func(fd protoreflect.FileDescriptor)
	add = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}
		seen[fd.Path()] = true
		// Emit dependencies first so the set stays topologically ordered.
		imps := fd.Imports()
		for i := 0; i < imps.Len(); i++ {
			add(imps.Get(i).FileDescriptor)
		}
		set.File = append(set.File, protodesc.ToFileDescriptorProto(fd))
	}
	for _, fd := range roots {
		add(fd)
	}
	return protodesc.NewFiles(set)
}
