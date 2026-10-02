package clientgen

import (
	"fmt"
	"go/format"
)

// OutputFile represents a generated file.
type OutputFile struct {
	Name    string
	Content []byte
}

// ClientGenBuilder configures and builds a ClientGen instance.
type ClientGenBuilder interface {
	WithSchema(schema *ModuleSchema) ClientGenBuilder
	WithOutputDir(dir string) ClientGenBuilder
	WithPackageName(name string) ClientGenBuilder
	WithIncludePrivate(include bool) ClientGenBuilder
	Build() (ClientGen, error)
}

// ClientGen generates Go client bindings from a resolved module schema.
type ClientGen interface {
	Generate() ([]OutputFile, error)
}

// NewClientGen returns a new ClientGenBuilder.
func NewClientGen() ClientGenBuilder {
	return &clientGen{}
}

type clientGen struct {
	schema         *ModuleSchema
	outputDir      string
	packageName    string
	includePrivate bool
}

func (g *clientGen) WithSchema(schema *ModuleSchema) ClientGenBuilder {
	g.schema = schema
	return g
}

func (g *clientGen) WithOutputDir(dir string) ClientGenBuilder {
	g.outputDir = dir
	return g
}

func (g *clientGen) WithPackageName(name string) ClientGenBuilder {
	g.packageName = name
	return g
}

func (g *clientGen) WithIncludePrivate(include bool) ClientGenBuilder {
	g.includePrivate = include
	return g
}

func (g *clientGen) Build() (ClientGen, error) {
	if g.schema == nil {
		return nil, fmt.Errorf("schema is required")
	}
	if g.packageName == "" {
		return nil, fmt.Errorf("package name is required")
	}
	return g, nil
}

func (g *clientGen) Generate() ([]OutputFile, error) {
	var files []OutputFile

	// Filter by visibility, then keep only types used by the generated API.
	schema := pruneUnusedTypes(g.filteredSchema())

	// Generate types
	typesContent, err := generateTypes(schema, g.packageName)
	if err != nil {
		return nil, fmt.Errorf("generating types: %w", err)
	}
	if len(typesContent) > 0 {
		formatted, err := gofmtBytes(typesContent)
		if err != nil {
			return nil, fmt.Errorf("formatting types: %w", err)
		}
		files = append(files, OutputFile{Name: "types_generated.go", Content: formatted})
	}

	// Generate BSATN encode/decode
	bsatnContent, err := generateBsatn(schema, g.packageName)
	if err != nil {
		return nil, fmt.Errorf("generating bsatn: %w", err)
	}
	if len(bsatnContent) > 0 {
		formatted, err := gofmtBytes(bsatnContent)
		if err != nil {
			return nil, fmt.Errorf("formatting bsatn: %w", err)
		}
		files = append(files, OutputFile{Name: "bsatn_generated.go", Content: formatted})
	}

	// Generate tables
	tablesContent, err := generateTables(schema, g.packageName)
	if err != nil {
		return nil, fmt.Errorf("generating tables: %w", err)
	}
	if len(tablesContent) > 0 {
		formatted, err := gofmtBytes(tablesContent)
		if err != nil {
			return nil, fmt.Errorf("formatting tables: %w", err)
		}
		files = append(files, OutputFile{Name: "tables_generated.go", Content: formatted})
	}

	// Generate reducers
	reducersContent, err := generateReducers(schema, g.packageName)
	if err != nil {
		return nil, fmt.Errorf("generating reducers: %w", err)
	}
	if len(reducersContent) > 0 {
		formatted, err := gofmtBytes(reducersContent)
		if err != nil {
			return nil, fmt.Errorf("formatting reducers: %w", err)
		}
		files = append(files, OutputFile{Name: "reducers_generated.go", Content: formatted})
	}

	// Generate procedures
	proceduresContent, err := generateProcedures(schema, g.packageName)
	if err != nil {
		return nil, fmt.Errorf("generating procedures: %w", err)
	}
	if len(proceduresContent) > 0 {
		formatted, err := gofmtBytes(proceduresContent)
		if err != nil {
			return nil, fmt.Errorf("formatting procedures: %w", err)
		}
		files = append(files, OutputFile{Name: "procedures_generated.go", Content: formatted})
	}

	// Generate views
	viewsContent, err := generateViews(schema, g.packageName)
	if err != nil {
		return nil, fmt.Errorf("generating views: %w", err)
	}
	if len(viewsContent) > 0 {
		formatted, err := gofmtBytes(viewsContent)
		if err != nil {
			return nil, fmt.Errorf("formatting views: %w", err)
		}
		files = append(files, OutputFile{Name: "views_generated.go", Content: formatted})
	}

	// Generate module bindings
	moduleContent, err := generateModule(schema, g.packageName)
	if err != nil {
		return nil, fmt.Errorf("generating module: %w", err)
	}
	if len(moduleContent) > 0 {
		formatted, err := gofmtBytes(moduleContent)
		if err != nil {
			return nil, fmt.Errorf("formatting module: %w", err)
		}
		files = append(files, OutputFile{Name: "module_generated.go", Content: formatted})
	}

	return files, nil
}

func (g *clientGen) filteredSchema() *ModuleSchema {
	filtered := &ModuleSchema{
		Typespace: g.schema.Typespace,
		Types:     g.schema.Types,
	}

	for _, t := range g.schema.Tables {
		if g.includePrivate || t.Access != "Private" {
			filtered.Tables = append(filtered.Tables, t)
		}
	}

	for _, r := range g.schema.Reducers {
		if r.Lifecycle == "" && (g.includePrivate || r.Visibility != "Private") {
			filtered.Reducers = append(filtered.Reducers, r)
		}
	}

	for _, p := range g.schema.Procedures {
		if g.includePrivate || p.Visibility != "Private" {
			filtered.Procedures = append(filtered.Procedures, p)
		}
	}

	for _, v := range g.schema.Views {
		if g.includePrivate || v.IsPublic {
			filtered.Views = append(filtered.Views, v)
		}
	}

	return filtered
}

func gofmtBytes(src []byte) ([]byte, error) {
	formatted, err := format.Source(src)
	if err != nil {
		return src, err
	}
	return formatted, nil
}
