package clientgen

import (
	"context"
	"fmt"
)

// SchemaExtractorBuilder configures a SchemaExtractor from a running SpacetimeDB server.
type SchemaExtractorBuilder interface {
	FromServer(serverURL, database, token string) SchemaExtractorBuilder
	WithSchemaVersion(v string) SchemaExtractorBuilder
	Build() (SchemaExtractor, error)
}

// SchemaExtractor extracts a module schema from a configured source.
type SchemaExtractor interface {
	Extract(ctx context.Context) (*ModuleSchema, error)
}

// NewSchemaExtractor returns a new SchemaExtractorBuilder.
func NewSchemaExtractor() SchemaExtractorBuilder {
	return &schemaExtractorBuilder{
		schemaVersion: "10",
	}
}

type schemaExtractorBuilder struct {
	serverURL     string
	database      string
	token         string
	schemaVersion string
}

func (b *schemaExtractorBuilder) FromServer(serverURL, database, token string) SchemaExtractorBuilder {
	b.serverURL = serverURL
	b.database = database
	b.token = token
	return b
}

func (b *schemaExtractorBuilder) WithSchemaVersion(v string) SchemaExtractorBuilder {
	b.schemaVersion = v
	return b
}

func (b *schemaExtractorBuilder) Build() (SchemaExtractor, error) {
	if b.serverURL != "" && b.database != "" {
		return &serverExtractor{
			serverURL:     b.serverURL,
			database:      b.database,
			token:         b.token,
			schemaVersion: b.schemaVersion,
		}, nil
	}
	return nil, fmt.Errorf("must provide server URL with database")
}
