package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display version information",
		Long:  `Display the version, commit, and build information for stdb-go.`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s version %s (commit: %s)\n", pkgName, version, commit)
		},
	}
}
