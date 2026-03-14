// stdb-go generates SpacetimeDB registration, BSATN encode/decode,
// table accessor, reducer dispatch, and module definition code from
// Go source files annotated with //stdb: comment directives.
//
// Usage:
//
//	go generate ./...
//
// With a go:generate directive in your module:
//
//	//go:generate go run go.digitalxero.dev/stdb-go
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
	rootCmd.AddCommand(newBuildCmd())
	rootCmd.AddCommand(newPublishCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newDevCmd())

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
		Use:           "stdb-go",
		Short:         "SpacetimeDB Go code generator",
		Long:          `stdb-go generates SpacetimeDB registration, BSATN encode/decode, table accessor, reducer dispatch, and module definition code from Go source files annotated with //stdb: comment directives.`,
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
		return fmt.Errorf("stdb-go: %w", err)
	}

	// Parse all Go source files in the directory.
	parsed, err := parseDirectory(absDir)
	if err != nil {
		return fmt.Errorf("stdb-go: parse error: %w", err)
	}

	// Analyze parsed declarations and resolve types.
	analyzed, err := analyze(parsed)
	if err != nil {
		return fmt.Errorf("stdb-go: analysis error: %w", err)
	}

	// Generate code.
	code, err := generate(analyzed)
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
