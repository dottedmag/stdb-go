package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"go.digitalxero.dev/stdb-go/internal/parser"
	"go.digitalxero.dev/stdb-go/internal/servergen"
)

func newGenerateServerCmd() *cobra.Command {
	var (
		dir    string
		output string
	)

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Generate server-side SpacetimeDB code",
		Long: `Generate server-side registration, BSATN encode/decode, table accessor,
reducer dispatch, and module definition code from Go source files annotated
with //stdb: comment directives.`,
		Example: `  # Generate in the current directory
  stdb-go generate server

  # Generate in a specific directory
  stdb-go generate server --dir=./mymodule

  # Generate with a custom output file name
  stdb-go generate server --output=custom_generated.go`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerateServer(dir, output)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", ".", "directory containing Go source files to process")
	cmd.Flags().StringVar(&output, "output", "stdb_generated.go", "output file name")

	return cmd
}

func runGenerateServer(dir, output string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("stdb-go: %w", err)
	}

	// Parse all Go source files in the directory.
	parsed, err := parser.ParseDirectory(absDir)
	if err != nil {
		return fmt.Errorf("stdb-go: parse error: %w", err)
	}

	// Analyze parsed declarations and resolve types.
	analyzed, err := servergen.Analyze(parsed)
	if err != nil {
		return fmt.Errorf("stdb-go: analysis error: %w", err)
	}

	// Generate code.
	code, err := servergen.Generate(analyzed)
	if err != nil {
		return fmt.Errorf("stdb-go: generation error: %w", err)
	}

	outputPath := filepath.Join(absDir, output)
	if err := os.WriteFile(outputPath, code, 0644); err != nil {
		return fmt.Errorf("stdb-go: write error: %w", err)
	}

	fmt.Fprintf(os.Stderr, "stdb-go: wrote %s\n", outputPath)
	return nil
}
