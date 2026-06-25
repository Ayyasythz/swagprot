package descsource

import (
	"context"
	"fmt"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// FromProtoFiles compiles the named .proto files (resolved against importPaths)
// and serves descriptors from the result. Standard imports such as
// google/protobuf/*.proto are provided automatically.
func FromProtoFiles(ctx context.Context, importPaths []string, files []string) (Source, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no .proto files given")
	}
	resolver := protocompile.WithStandardImports(&protocompile.SourceResolver{
		ImportPaths: importPaths,
	})
	compiler := protocompile.Compiler{Resolver: resolver}

	compiled, err := compiler.Compile(ctx, files...)
	if err != nil {
		return nil, fmt.Errorf("compile protos: %w", err)
	}

	roots := make([]protoreflect.FileDescriptor, 0, len(compiled))
	for _, f := range compiled {
		roots = append(roots, f)
	}
	reg, err := registryFromRoots(roots)
	if err != nil {
		return nil, err
	}
	return &staticSource{files: reg}, nil
}
