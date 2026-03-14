package clientgen

import (
	"context"
	"fmt"
)

// SchemaExtractorBuilder configures a SchemaExtractor from either a WASM binary
// or a running SpacetimeDB server.
type SchemaExtractorBuilder interface {
	FromWasm(binPath string) SchemaExtractorBuilder
	FromServer(serverURL, database, token string) SchemaExtractorBuilder
	Build() (SchemaExtractor, error)
}

// SchemaExtractor extracts a module schema from a configured source.
type SchemaExtractor interface {
	Extract(ctx context.Context) (*ModuleSchema, error)
}

// NewSchemaExtractor returns a new SchemaExtractorBuilder.
func NewSchemaExtractor() SchemaExtractorBuilder {
	return &schemaExtractorBuilder{}
}

type schemaExtractorBuilder struct {
	wasmPath  string
	serverURL string
	database  string
	token     string
}

func (b *schemaExtractorBuilder) FromWasm(binPath string) SchemaExtractorBuilder {
	b.wasmPath = binPath
	return b
}

func (b *schemaExtractorBuilder) FromServer(serverURL, database, token string) SchemaExtractorBuilder {
	b.serverURL = serverURL
	b.database = database
	b.token = token
	return b
}

func (b *schemaExtractorBuilder) Build() (SchemaExtractor, error) {
	if b.wasmPath != "" {
		return &wasmExtractor{binPath: b.wasmPath}, nil
	}
	if b.serverURL != "" && b.database != "" {
		return &serverExtractor{
			serverURL: b.serverURL,
			database:  b.database,
			token:     b.token,
		}, nil
	}
	return nil, fmt.Errorf("must provide either WASM binary path or server URL with database")
}
