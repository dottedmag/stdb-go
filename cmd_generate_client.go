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
		binPath        string
		server         string
		database       string
		token          string
		packageName    string
		includePrivate bool
	)

	cmd := &cobra.Command{
		Use:   "client",
		Short: "Generate client-side Go bindings from a SpacetimeDB module schema",
		Long: `Generate type-safe Go client bindings from a SpacetimeDB module schema.

The schema can be extracted from either a compiled WASM binary (via the
spacetime CLI) or from a running SpacetimeDB server instance.

You must provide at least one schema source:
  --bin-path    Path to a compiled WASM binary
  -s/--server   SpacetimeDB server URL (with -d/--database)`,
		Example: `  # Generate from a WASM binary
  stdb-go generate client --bin-path=module.wasm --out-dir=./bindings

  # Generate from a running server
  stdb-go generate client -d my-database --out-dir=./bindings

  # Generate with a custom package name
  stdb-go generate client --bin-path=module.wasm --out-dir=./bindings --package=mymodule

  # Include private tables and reducers
  stdb-go generate client --bin-path=module.wasm --out-dir=./bindings --include-private`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerateClient(outDir, binPath, server, database, token, packageName, includePrivate)
		},
	}

	cmd.Flags().StringVar(&outDir, "out-dir", "module_bindings", "output directory for generated files")
	cmd.Flags().StringVar(&binPath, "bin-path", "", "path to compiled WASM binary")
	cmd.Flags().StringVarP(&server, "server", "s", "", "SpacetimeDB server URL (default: from config or http://localhost:3000)")
	cmd.Flags().StringVarP(&database, "database", "d", "", "database name or identity")
	cmd.Flags().StringVar(&token, "token", "", "auth token (default: from env or cli.toml)")
	cmd.Flags().StringVar(&packageName, "package", "", "Go package name (default: derived from out-dir)")
	cmd.Flags().BoolVar(&includePrivate, "include-private", false, "include private tables and reducers")

	return cmd
}

func runGenerateClient(outDir, binPath, server, database, token, packageName string, includePrivate bool) error {
	// Validate that at least one schema source is provided
	if binPath == "" && database == "" {
		return fmt.Errorf("generate client: must provide either --bin-path or --database/-d")
	}

	// Derive package name from output directory if not specified
	if packageName == "" {
		packageName = filepath.Base(outDir)
	}

	// Build schema extractor
	extractorBuilder := clientgen.NewSchemaExtractor()

	if binPath != "" {
		absBinPath, err := filepath.Abs(binPath)
		if err != nil {
			return fmt.Errorf("generate client: %w", err)
		}
		extractorBuilder = extractorBuilder.FromWasm(absBinPath)
	} else {
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

		extractorBuilder = extractorBuilder.FromServer(resolvedServer, resolvedDB, resolvedToken)
	}

	extractor, err := extractorBuilder.Build()
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
