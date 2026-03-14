package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.digitalxero.dev/stdb-go/internal/scaffold"
)

func newInitCmd() *cobra.Command {
	var (
		name        string
		module      string
		projectType string
		dir         string
	)

	cmd := &cobra.Command{
		Use:   "init [name]",
		Short: "Initialize a new SpacetimeDB project",
		Long: `Initialize a new SpacetimeDB project with boilerplate code.

Scaffolds a project structure for server modules, Go clients, or
full-stack projects with both server and client.`,
		Example: `  # Create a server module
  stdb-go init myproject

  # Create a client project
  stdb-go init myproject --type client

  # Create a full-stack project with custom module path
  stdb-go init myproject --type fullstack --module github.com/me/myproject

  # Create in a specific directory
  stdb-go init myproject --dir /path/to/project`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				name = args[0]
			}
			if name == "" {
				return fmt.Errorf("project name is required (as argument or --name flag)")
			}

			s, err := scaffold.NewScaffoldBuilder().
				WithName(name).
				WithModule(module).
				WithType(scaffold.ProjectType(projectType)).
				WithDir(dir).
				Build()
			if err != nil {
				return err
			}

			if err := s.Generate(); err != nil {
				return err
			}

			outDir := dir
			if outDir == "" {
				outDir = "./" + name
			}
			fmt.Fprintf(os.Stderr, "init: created %s project in %s\n", projectType, outDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "project name (alternative to positional argument)")
	cmd.Flags().StringVar(&module, "module", "", "Go module path (defaults to project name)")
	cmd.Flags().StringVar(&projectType, "type", "server", "project type: server, client, or fullstack")
	cmd.Flags().StringVar(&dir, "dir", "", "output directory (defaults to ./<name>)")

	return cmd
}
