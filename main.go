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
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := newRootCmd()
	rootCmd.AddCommand(newGenerateCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newUpgradeCmd())
	rootCmd.AddCommand(newBuildCmd())
	rootCmd.AddCommand(newPublishCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newDevCmd())
	rootCmd.AddCommand(newSkillsCmd())

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
		Use:          "stdb-go",
		Short:        "SpacetimeDB Go code generator",
		Long:         `stdb-go generates SpacetimeDB registration, BSATN encode/decode, table accessor, reducer dispatch, and module definition code from Go source files annotated with //stdb: comment directives.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Backwards compatibility: bare `stdb-go` runs server codegen
			return runGenerateServer(dir, output)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", ".", "directory containing Go source files to process")
	cmd.Flags().StringVar(&output, "output", "stdb_generated.go", "output file name")

	return cmd
}
