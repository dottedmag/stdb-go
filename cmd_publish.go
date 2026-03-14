package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"go.digitalxero.dev/stdb-go/internal/publish"
)

func newPublishCmd() *cobra.Command {
	var (
		dir           string
		database      string
		server        string
		token         string
		wasmFile      string
		clearDatabase bool
		skipBuild     bool
		autoConfirm   bool
		wasiShim      bool
	)

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Build and publish a SpacetimeDB WASM module",
		Long: `Publish builds a Go SpacetimeDB module and deploys it to a running
SpacetimeDB instance. It reads configuration from spacetime.json and
~/.config/spacetime/cli.toml for defaults.`,
		Example: `  # Publish to a local SpacetimeDB instance
  stdb-go publish -d my-database

  # Publish to a remote server
  stdb-go publish -d my-database -s https://spacetimedb.example.com

  # Publish a pre-built WASM file
  stdb-go publish -d my-database --wasm-file=module.wasm --skip-build

  # Clear the database before publishing
  stdb-go publish -d my-database --clear-database`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPublish(dir, database, server, token, wasmFile, clearDatabase, skipBuild, autoConfirm, wasiShim)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", ".", "module directory")
	cmd.Flags().StringVarP(&database, "database", "d", "", "database name or identity")
	cmd.Flags().StringVarP(&server, "server", "s", "", "server URL (default: from config or http://localhost:3000)")
	cmd.Flags().StringVar(&token, "token", "", "auth token (default: from env or cli.toml)")
	cmd.Flags().StringVar(&wasmFile, "wasm-file", "", "use pre-built WASM file instead of building")
	cmd.Flags().BoolVar(&clearDatabase, "clear-database", false, "clear database before publishing")
	cmd.Flags().BoolVar(&skipBuild, "skip-build", false, "skip build step (requires --wasm-file)")
	cmd.Flags().BoolVarP(&autoConfirm, "yes", "y", false, "auto-confirm breaking changes")
	cmd.Flags().BoolVar(&wasiShim, "wasi-shim", true, "rewrite WASI imports with local stubs")

	return cmd
}

func runPublish(dir, database, server, token, wasmFile string, clearDatabase, skipBuild, autoConfirm, wasiShim bool) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	// Load configs
	spacetimeCfg, err := publish.LoadSpacetimeConfig(absDir)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	cliCfg, err := publish.LoadCLIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "publish: warning: %v\n", err)
	}

	// Resolve settings
	resolvedDB := publish.ResolveDatabase(database, spacetimeCfg)
	if resolvedDB == "" {
		return fmt.Errorf("publish: database name is required (use --database flag or spacetime.json)")
	}

	resolvedServer := publish.ResolveServer(server, spacetimeCfg, cliCfg)
	resolvedToken := publish.ResolveToken(token, cliCfg)

	fmt.Fprintf(os.Stderr, "publish: database=%s server=%s\n", resolvedDB, resolvedServer)

	// Determine WASM file path
	wasmPath := wasmFile
	if wasmPath == "" {
		wasmPath = filepath.Join(absDir, "module.wasm")
	}

	// Build unless skipped
	if !skipBuild {
		if err := runBuild(absDir, wasmPath, true, true, wasiShim); err != nil {
			return fmt.Errorf("publish: %w", err)
		}
	} else if wasmFile == "" {
		return fmt.Errorf("publish: --skip-build requires --wasm-file")
	}

	// Read WASM bytes
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return fmt.Errorf("publish: reading WASM file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "publish: WASM module size: %d bytes\n", len(wasmBytes))

	// Build publisher
	pub, err := publish.NewPublisherBuilder().
		WithServer(resolvedServer).
		WithDatabase(resolvedDB).
		WithToken(resolvedToken).
		WithClearDatabase(clearDatabase).
		WithAutoConfirm(autoConfirm).
		Build()
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	ctx := cmd_context()

	// Pre-publish check
	fmt.Fprintf(os.Stderr, "publish: checking for migrations...\n")
	preResult, err := pub.PrePublish(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("publish: pre-publish check failed: %w", err)
	}

	if preResult != nil {
		if preResult.ManualMigrate != nil {
			mm := preResult.ManualMigrate
			fmt.Fprintf(os.Stderr, "publish: manual migration required:\n")
			fmt.Fprintf(os.Stderr, "  %s\n", mm.Summary)
			for _, detail := range mm.Details {
				fmt.Fprintf(os.Stderr, "  - %s\n", detail)
			}
			if mm.HasErrors && !autoConfirm {
				return fmt.Errorf("publish: breaking changes detected; use --yes to confirm or --clear-database to start fresh")
			}
		}
		if preResult.AutoMigrate != nil {
			fmt.Fprintf(os.Stderr, "publish: auto-migration plan:\n%s\n", preResult.AutoMigrate.MigrationPlan)
		}
	}

	// Publish
	fmt.Fprintf(os.Stderr, "publish: publishing module...\n")
	result, err := pub.Publish(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	if result.PermissionDenied != nil {
		return fmt.Errorf("publish: permission denied for database %q", result.PermissionDenied.Name)
	}

	if result.Success != nil {
		s := result.Success
		op := "updated"
		if s.Op == "Created" {
			op = "created"
		}
		fmt.Fprintf(os.Stderr, "publish: %s database %q (identity: %s)\n", op, resolvedDB, s.DatabaseIdentity)
		if s.Domain != nil {
			fmt.Fprintf(os.Stderr, "publish: domain: %s\n", *s.Domain)
		}
	}

	return nil
}

// cmd_context returns a context that respects interrupt signals.
// For now, returns a background context; the root command's context
// could be threaded through if desired.
func cmd_context() context.Context {
	return context.Background()
}
