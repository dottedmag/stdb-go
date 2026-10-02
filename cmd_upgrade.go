package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/dottedmag/stdb-go/internal/upgrade"
)

func newUpgradeCmd() *cobra.Command {
	var (
		listVersions  bool
		targetVersion string
	)

	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"self-update"},
		Short:   "Upgrade stdb-go to a newer version",
		Long: `Upgrade stdb-go to a newer version from GitLab releases.

By default, upgrades to the latest available version. Use --version to specify
a target version, or --list to see available versions.`,
		Example: `  # Upgrade to the latest version
  stdb-go upgrade

  # List available versions
  stdb-go upgrade --list

  # Upgrade to a specific version
  stdb-go upgrade --version v0.2.0
  stdb-go upgrade -v v0.2.0`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Get GitLab token from environment if available
			gitlabToken := os.Getenv("GITLAB_TOKEN")
			if gitlabToken == "" {
				gitlabToken = os.Getenv("GL_TOKEN")
			}

			// Build upgrader
			upgrader, err := upgrade.NewUpgraderBuilder().
				WithCurrentVersion(version).
				WithBinaryName(pkgName).
				WithStdout(os.Stdout).
				WithGitLabToken(gitlabToken).
				Build(ctx)
			if err != nil {
				return fmt.Errorf("failed to initialize upgrader: %w", err)
			}

			// List versions mode
			if listVersions {
				return runListAvailableVersions(ctx, upgrader)
			}

			// Upgrade mode
			result, err := upgrader.Upgrade(ctx, targetVersion)
			if err != nil {
				if errors.Is(err, upgrade.ErrSameVersion) {
					fmt.Printf("You are already running version %s\n", version)
					return nil
				}
				return fmt.Errorf("upgrade failed: %w", err)
			}

			if result.Success() {
				fmt.Println(result.Message())
				fmt.Println("\nPlease restart stdb-go to use the new version.")
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&listVersions, "list", "l", false, "List available versions")
	cmd.Flags().StringVarP(&targetVersion, "version", "v", "", "Target version to upgrade to")

	return cmd
}

func runListAvailableVersions(ctx context.Context, upgrader upgrade.Upgrader) error {
	// Check if context is cancelled
	select {
	case <-ctx.Done():
		return fmt.Errorf("operation cancelled")
	default:
	}

	versions, err := upgrader.ListVersions(ctx)
	if err != nil {
		return fmt.Errorf("failed to list versions: %w", err)
	}

	if len(versions) == 0 {
		fmt.Println("No versions available.")
		return nil
	}

	fmt.Printf("Current version: %s\n\n", version)
	fmt.Println("Available versions:")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintf(w, "VERSION\tPUBLISHED\tAVAILABLE\n")
	_, _ = fmt.Fprintf(w, "-------\t---------\t---------\n")

	for _, v := range versions {
		available := "Yes"
		if !v.HasAsset() {
			available = "No (no binary)"
		}
		published := v.PublishedAt()
		if len(published) > 10 {
			published = published[:10] // Just the date part
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", v.Version(), published, available)
	}

	_ = w.Flush()

	return nil
}
