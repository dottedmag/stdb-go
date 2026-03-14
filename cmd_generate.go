package main

import (
	"github.com/spf13/cobra"
)

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate SpacetimeDB code",
		Long: `Generate runs code generation for SpacetimeDB modules.

When called with no subcommand, it runs server codegen (same as 'generate server').`,
	}

	// Add subcommands
	cmd.AddCommand(newGenerateServerCmd())
	cmd.AddCommand(newGenerateClientCmd())

	return cmd
}
