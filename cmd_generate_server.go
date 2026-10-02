package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dottedmag/stdb-go/internal/parser"
	"github.com/dottedmag/stdb-go/internal/servergen"
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

	// Parse all Go packages under the module root (nested directories included).
	parsed, err := parser.ParseDirectory(absDir)
	if err != nil {
		return fmt.Errorf("stdb-go: parse error: %w", err)
	}

	// Analyze parsed declarations and resolve types.
	analyzed, err := servergen.Analyze(parsed)
	if err != nil {
		return fmt.Errorf("stdb-go: analysis error: %w", err)
	}

	// Generate code (one or more files for multi-package modules).
	files, err := servergen.GenerateAll(analyzed)
	if err != nil {
		return fmt.Errorf("stdb-go: generation error: %w", err)
	}

	for _, f := range files {
		// Honor --output only for the primary single-package artifact name.
		rel := f.RelPath
		if !analyzed.MultiPackage && output != "" && output != "stdb_generated.go" {
			// Custom output name for flat modules.
			if filepath.Base(rel) == "stdb_generated.go" {
				rel = output
			}
		}
		outputPath := filepath.Join(absDir, rel)
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			return fmt.Errorf("stdb-go: mkdir: %w", err)
		}
		if err := os.WriteFile(outputPath, f.Content, 0644); err != nil {
			return fmt.Errorf("stdb-go: write error: %w", err)
		}
		fmt.Fprintf(os.Stderr, "stdb-go: wrote %s\n", outputPath)
	}
	return nil
}
