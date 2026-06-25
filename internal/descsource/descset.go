package descsource

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// FromDescriptorSet loads descriptors from a binary FileDescriptorSet file,
// e.g. one produced by `protoc --include_imports --descriptor_set_out=set.binpb`.
func FromDescriptorSet(path string) (Source, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read descriptor set: %w", err)
	}
	set := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(raw, set); err != nil {
		return nil, fmt.Errorf("parse descriptor set %q: %w", path, err)
	}
	if len(set.File) == 0 {
		return nil, fmt.Errorf("descriptor set %q contains no files", path)
	}
	return newStaticFromSet(set)
}
