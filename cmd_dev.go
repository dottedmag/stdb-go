package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"go.digitalxero.dev/stdb-go/internal/dev"
	"go.digitalxero.dev/stdb-go/internal/publish"
)

func newDevCmd() *cobra.Command {
	var (
		dir           string
		database      string
		server        string
		token         string
		debounce      time.Duration
		clearDatabase bool
		wasiShim      bool
		clientCmd     string
	)

	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Watch for changes and auto-build+publish to SpacetimeDB",
		Long: `Dev watches for Go file changes in the module directory, automatically
rebuilds the WASM module, and publishes it to a running SpacetimeDB instance.

Start SpacetimeDB separately (e.g., via Docker), then run this command
to get a live-reload development workflow.`,
		Example: `  # Basic dev workflow
  stdb-go dev -d my-database

  # With a remote server
  stdb-go dev -d my-database -s https://spacetimedb.example.com

  # Run a client alongside
  stdb-go dev -d my-database --client-cmd="go run ./cmd/client"

  # Custom debounce
  stdb-go dev -d my-database --debounce=1s`,
		RunE: func(cmd *cobra.Command, args []string) error {
			absDir, err := filepath.Abs(dir)
			if err != nil {
				return fmt.Errorf("dev: %w", err)
			}

			// Resolve database, server, token from flags/config.
			spacetimeCfg, err := publish.LoadSpacetimeConfig(absDir)
			if err != nil {
				return fmt.Errorf("dev: %w", err)
			}

			cliCfg, err := publish.LoadCLIConfig()
			if err != nil {
				fmt.Fprintf(os.Stderr, "dev: warning: %v\n", err)
			}

			resolvedDB := publish.ResolveDatabase(database, spacetimeCfg)
			if resolvedDB == "" {
				return fmt.Errorf("dev: database name is required (use --database flag or spacetime.json)")
			}

			resolvedServer := publish.ResolveServer(server, spacetimeCfg, cliCfg)
			resolvedToken := publish.ResolveToken(token, cliCfg)

			fmt.Fprintf(os.Stderr, "dev: database=%s server=%s\n", resolvedDB, resolvedServer)

			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			runner, err := dev.NewDevRunner().
				WithDir(absDir).
				WithDatabase(resolvedDB).
				WithServer(resolvedServer).
				WithToken(resolvedToken).
				WithDebounce(debounce).
				WithClearDatabase(clearDatabase).
				WithWasiShim(wasiShim).
				WithClientCmd(clientCmd).
				WithBuildFunc(runBuild).
				WithPublishFunc(runPublish).
				Build()
			if err != nil {
				return err
			}

			return runner.Run(ctx)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", ".", "module directory")
	cmd.Flags().StringVarP(&database, "database", "d", "", "database name (from flag or spacetime.json)")
	cmd.Flags().StringVarP(&server, "server", "s", "", "server URL (default: from config or http://localhost:3000)")
	cmd.Flags().StringVar(&token, "token", "", "auth token")
	cmd.Flags().DurationVar(&debounce, "debounce", 500*time.Millisecond, "debounce duration for file watching")
	cmd.Flags().BoolVar(&clearDatabase, "clear-database", true, "clear database on initial publish")
	cmd.Flags().BoolVar(&wasiShim, "wasi-shim", true, "rewrite WASI imports with local stubs")
	cmd.Flags().StringVar(&clientCmd, "client-cmd", "", "client command to run alongside")

	return cmd
}
