// stdb-gen generates SpacetimeDB registration, BSATN encode/decode,
// table accessor, reducer dispatch, and module definition code from
// Go source files annotated with //stdb: comment directives.
//
// Usage:
//
//	go generate ./...
//
// With a go:generate directive in your module:
//
//	//go:generate go run go.digitalxero.dev/stdb-gen
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := newRootCmd()
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newUpgradeCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var (
		dir    string
		output string
	)

	cmd := &cobra.Command{
		Use:           "stdb-gen",
		Short:         "SpacetimeDB Go code generator",
		Long:          `stdb-gen generates SpacetimeDB registration, BSATN encode/decode, table accessor, reducer dispatch, and module definition code from Go source files annotated with //stdb: comment directives.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGenerate(dir, output)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", ".", "directory containing Go source files to process")
	cmd.Flags().StringVar(&output, "output", "stdb_generated.go", "output file name")

	return cmd
}

func runGenerate(dir, output string) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("stdb-gen: %w", err)
	}

	// Parse all Go source files in the directory.
	parsed, err := parseDirectory(absDir)
	if err != nil {
		return fmt.Errorf("stdb-gen: parse error: %w", err)
	}

	// Analyze parsed declarations and resolve types.
	analyzed, err := analyze(parsed)
	if err != nil {
		return fmt.Errorf("stdb-gen: analysis error: %w", err)
	}

	// Generate code.
	code, err := generate(analyzed)
	if err != nil {
		return fmt.Errorf("stdb-gen: generation error: %w", err)
	}

	outputPath := filepath.Join(absDir, output)
	if err := os.WriteFile(outputPath, code, 0644); err != nil {
		return fmt.Errorf("stdb-gen: write error: %w", err)
	}

	fmt.Fprintf(os.Stderr, "stdb-gen: wrote %s\n", outputPath)
	return nil
}
