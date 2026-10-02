package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dottedmag/stdb-go/internal/publish"
)

func newPublishCmd() *cobra.Command {
	var (
		dir           string
		database      string
		server        string
		token         string
		wasmFile      string
		clearDatabase bool
		deleteData    string
		breakClients  bool
		numReplicas   uint
		parent        string
		organization  string
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

  # Allow a schema change that breaks existing clients (e.g. a new default= column)
  stdb-go publish -d my-database --break-clients

  # Clear the database before publishing
  stdb-go publish -d my-database --clear-database`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPublish(publishArgs{
				dir:           dir,
				database:      database,
				server:        server,
				token:         token,
				wasmFile:      wasmFile,
				clearDatabase: clearDatabase,
				deleteData:    deleteData,
				breakClients:  breakClients,
				numReplicas:   numReplicas,
				parent:        parent,
				organization:  organization,
				skipBuild:     skipBuild,
				autoConfirm:   autoConfirm,
				wasiShim:      wasiShim,
			})
		},
	}

	cmd.Flags().StringVar(&dir, "dir", ".", "module directory")
	cmd.Flags().StringVarP(&database, "database", "d", "", "database name or identity")
	cmd.Flags().StringVarP(&server, "server", "s", "", "server URL (default: from config or http://localhost:3000)")
	cmd.Flags().StringVar(&token, "token", "", "auth token (default: from env or cli.toml)")
	cmd.Flags().StringVar(&wasmFile, "wasm-file", "", "use pre-built WASM file instead of building")
	cmd.Flags().BoolVar(&clearDatabase, "clear-database", false, "destroy all data before publishing (alias for --delete-data=always)")
	cmd.Flags().StringVar(&deleteData, "delete-data", "never", "when to destroy data on a migration: always|on-conflict|never")
	cmd.Flags().BoolVar(&breakClients, "break-clients", false, "allow a migration that breaks existing clients (e.g. a new default= column)")
	cmd.Flags().UintVar(&numReplicas, "num-replicas", 0, "number of replicas the database should have (0 = server default)")
	cmd.Flags().StringVar(&parent, "parent", "", "parent database (name or identity); only applied when creating a database")
	cmd.Flags().StringVar(&organization, "organization", "", "organization (name or identity); only applied when creating a database")
	cmd.Flags().StringVar(&organization, "org", "", "alias for --organization")
	cmd.Flags().BoolVar(&skipBuild, "skip-build", false, "skip build step (requires --wasm-file)")
	cmd.Flags().BoolVarP(&autoConfirm, "yes", "y", false, "auto-confirm breaking changes and migrations")
	cmd.Flags().BoolVar(&wasiShim, "wasi-shim", true, "rewrite WASI imports with local stubs")

	return cmd
}

// publishArgs bundles the resolved CLI flags for a publish run.
type publishArgs struct {
	dir           string
	database      string
	server        string
	token         string
	wasmFile      string
	clearDatabase bool
	deleteData    string
	breakClients  bool
	numReplicas   uint
	parent        string
	organization  string
	skipBuild     bool
	autoConfirm   bool
	wasiShim      bool
}

func runPublish(a publishArgs) error {
	absDir, err := filepath.Abs(a.dir)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	// Resolve the data-deletion mode (--clear-database is an alias for always).
	clearMode := a.deleteData
	if a.clearDatabase {
		clearMode = clearModeAlways
	}
	switch clearMode {
	case clearModeAlways, clearModeOnConflict, clearModeNever:
	default:
		return fmt.Errorf("publish: invalid --delete-data %q (want always|on-conflict|never)", a.deleteData)
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
	resolvedDB := publish.ResolveDatabase(a.database, spacetimeCfg)
	if resolvedDB == "" {
		return fmt.Errorf("publish: database name is required (use --database flag or spacetime.json)")
	}

	resolvedServer := publish.ResolveServer(a.server, spacetimeCfg, cliCfg)
	fmt.Fprintf(os.Stderr, "publish: database=%s server=%s\n", resolvedDB, resolvedServer)

	ctx := cmd_context()
	resolvedToken, err := resolveOrCreateToken(ctx, resolvedServer, a.token, cliCfg)
	if err != nil {
		return err
	}

	// Determine WASM file path
	wasmPath := a.wasmFile
	if wasmPath == "" {
		wasmPath = filepath.Join(absDir, "module.wasm")
	}

	// Build unless skipped
	if !a.skipBuild {
		if err := runBuild(absDir, wasmPath, true, true, a.wasiShim); err != nil {
			return fmt.Errorf("publish: %w", err)
		}
	} else if a.wasmFile == "" {
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
		Build()
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	// Resolve per-publish options (migration policy, data deletion, creation params).
	opts := publish.PublishOptions{
		Parent:       a.parent,
		Organization: a.organization,
	}
	if a.numReplicas > 0 {
		n := a.numReplicas
		opts.NumReplicas = &n
	}

	if clearMode == clearModeAlways {
		// Wiping data sidesteps the migration check entirely.
		opts.Clear = true
	} else if err := resolveMigration(ctx, pub, wasmBytes, clearMode, a.breakClients, a.autoConfirm, &opts); err != nil {
		return err
	}

	// Publish
	fmt.Fprintf(os.Stderr, "publish: publishing module...\n")
	result, err := pub.Publish(ctx, wasmBytes, opts)
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

// resolveOrCreateToken returns the auth token, minting and persisting a new one
// when none is configured. This makes publishing zero-setup: the first run on a
// fresh machine creates an identity on the target server and saves its token to
// cli.toml, and subsequent runs reuse it (so the same identity owns the database).
func resolveOrCreateToken(ctx context.Context, server, flagToken string, cliCfg *publish.CLIConfig) (string, error) {
	if tok := publish.ResolveToken(flagToken, cliCfg); tok != "" {
		return tok, nil
	}

	fmt.Fprintf(os.Stderr, "publish: no auth token found; creating a new identity on %s\n", server)
	identity, token, err := publish.CreateIdentity(ctx, server)
	if err != nil {
		return "", fmt.Errorf("publish: creating identity: %w", err)
	}
	path, err := publish.SaveSpacetimeToken(token)
	if err != nil {
		return "", fmt.Errorf("publish: saving token: %w", err)
	}
	fmt.Fprintf(os.Stderr, "publish: saved token for identity %s to %s\n", identity, path)
	return token, nil
}

// Data-deletion modes for --delete-data (and the --clear-database alias).
const (
	clearModeAlways     = "always"
	clearModeOnConflict = "on-conflict"
	clearModeNever      = "never"
)

// resolveMigration runs the pre-publish check and decides how to proceed,
// updating opts (clear / break-clients policy + token) or returning an error
// that aborts the publish. It mirrors the official CLI's
// apply_pre_publish_if_needed. clearMode is never "always" here (that case skips
// the check entirely).
func resolveMigration(ctx context.Context, pub publish.Publisher, wasmBytes []byte, clearMode string, breakClients, autoConfirm bool, opts *publish.PublishOptions) error {
	fmt.Fprintf(os.Stderr, "publish: checking for breaking changes...\n")
	pre, err := pub.PrePublish(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("publish: pre-publish check failed: %w", err)
	}
	if pre == nil {
		// New database (404): nothing to migrate.
		return nil
	}

	switch {
	case pre.ManualMigrate != nil:
		mm := pre.ManualMigrate
		fmt.Fprintf(os.Stderr, "publish: %s\n", mm.Reason)
		if mm.MajorVersionUpgrade && !autoConfirm {
			return fmt.Errorf("publish: this is a major version upgrade; re-run with --yes to confirm")
		}
		if clearMode == clearModeNever {
			return fmt.Errorf("publish: this change requires manual migration; existing data must be deleted. " +
				"Re-run with --delete-data=on-conflict (or --clear-database) to wipe and republish")
		}
		fmt.Fprintf(os.Stderr, "publish: clearing data due to --delete-data=on-conflict\n")
		opts.Clear = true

	case pre.AutoMigrate != nil:
		am := pre.AutoMigrate
		if am.MigratePlan != "" {
			fmt.Fprintf(os.Stderr, "%s\n", am.MigratePlan)
		}
		if am.MajorVersionUpgrade && !autoConfirm {
			return fmt.Errorf("publish: this is a major version upgrade; re-run with --yes to confirm")
		}
		if am.BreakClients {
			if !breakClients && !autoConfirm {
				return fmt.Errorf("publish: these changes will BREAK existing clients; re-run with --break-clients to proceed")
			}
			opts.Policy = "BreakClients"
			opts.MigrationToken = am.QueryToken()
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
