package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"go.digitalxero.dev/stdb-go/internal/clientgen"
	"go.digitalxero.dev/stdb-go/internal/publish"
)

func newGenerateClientCmd() *cobra.Command {
	var (
		outDir         string
		server         string
		database       string
		token          string
		packageName    string
		schemaVersion  string
		includePrivate bool
	)

	cmd := &cobra.Command{
		Use:   "client",
		Short: "Generate client-side Go bindings from a SpacetimeDB module schema",
		Long: `Generate type-safe Go client bindings from a SpacetimeDB module schema.

The schema is fetched from a running SpacetimeDB server instance.

Required:
  -d/--database   Database name or identity`,
		Example: `  # Generate from a running server
  stdb-go generate client -d my-database --out-dir=./bindings

  # Generate with a custom package name
  stdb-go generate client -d my-database --out-dir=./bindings --package=mymodule

  # Include private tables and reducers
  stdb-go generate client -d my-database --out-dir=./bindings --include-private

  # Specify schema version
  stdb-go generate client -d my-database --out-dir=./bindings --schema-version=10`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerateClient(outDir, server, database, token, packageName, schemaVersion, includePrivate)
		},
	}

	cmd.Flags().StringVar(&outDir, "out-dir", "module_bindings", "output directory for generated files")
	cmd.Flags().StringVarP(&server, "server", "s", "", "SpacetimeDB server URL (default: from config or http://localhost:3000)")
	cmd.Flags().StringVarP(&database, "database", "d", "", "database name or identity")
	cmd.Flags().StringVar(&token, "token", "", "auth token (default: from env or cli.toml)")
	cmd.Flags().StringVar(&packageName, "package", "", "Go package name (default: derived from out-dir)")
	cmd.Flags().StringVar(&schemaVersion, "schema-version", "10", "schema version for the server API")
	cmd.Flags().BoolVar(&includePrivate, "include-private", false, "include private tables and reducers")

	_ = cmd.MarkFlagRequired("database")

	return cmd
}

func runGenerateClient(outDir, server, database, token, packageName, schemaVersion string, includePrivate bool) error {
	// Derive package name from output directory if not specified
	if packageName == "" {
		packageName = filepath.Base(outDir)
	}

	// Resolve server/database/token from flags and config
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("generate client: %w", err)
	}

	spacetimeCfg, err := publish.LoadSpacetimeConfig(cwd)
	if err != nil {
		return fmt.Errorf("generate client: %w", err)
	}

	cliCfg, err := publish.LoadCLIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate client: warning: %v\n", err)
	}

	resolvedServer := publish.ResolveServer(server, spacetimeCfg, cliCfg)
	resolvedDB := publish.ResolveDatabase(database, spacetimeCfg)
	resolvedToken := publish.ResolveToken(token, cliCfg)

	if resolvedDB == "" {
		return fmt.Errorf("generate client: database name is required (use --database flag or spacetime.json)")
	}

	// Build schema extractor
	extractor, err := clientgen.NewSchemaExtractor().
		FromServer(resolvedServer, resolvedDB, resolvedToken).
		WithSchemaVersion(schemaVersion).
		Build()
	if err != nil {
		return fmt.Errorf("generate client: %w", err)
	}

	// Extract schema
	fmt.Fprintf(os.Stderr, "generate client: extracting schema...\n")
	schema, err := extractor.Extract(cmd_context())
	if err != nil {
		return fmt.Errorf("generate client: schema extraction failed: %w", err)
	}

	// Generate client code
	fmt.Fprintf(os.Stderr, "generate client: generating code...\n")
	gen, err := clientgen.NewClientGen().
		WithSchema(schema).
		WithOutputDir(outDir).
		WithPackageName(packageName).
		WithIncludePrivate(includePrivate).
		Build()
	if err != nil {
		return fmt.Errorf("generate client: %w", err)
	}

	files, err := gen.Generate()
	if err != nil {
		return fmt.Errorf("generate client: code generation failed: %w", err)
	}

	// Write output files
	absOutDir, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("generate client: %w", err)
	}

	if err := os.MkdirAll(absOutDir, 0755); err != nil {
		return fmt.Errorf("generate client: creating output directory: %w", err)
	}

	for _, f := range files {
		outputPath := filepath.Join(absOutDir, f.Name)
		if err := os.WriteFile(outputPath, f.Content, 0644); err != nil {
			return fmt.Errorf("generate client: writing %s: %w", f.Name, err)
		}
		fmt.Fprintf(os.Stderr, "generate client: wrote %s\n", outputPath)
	}

	fmt.Fprintf(os.Stderr, "generate client: done (%d files)\n", len(files))
	return nil
}
