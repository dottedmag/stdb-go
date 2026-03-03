package upgrade

import (
	"context"
	"io"
)

// UpgraderBuilder builds an Upgrader using the builder pattern.
type UpgraderBuilder interface {
	// WithCurrentVersion sets the current version of the binary.
	WithCurrentVersion(version string) UpgraderBuilder
	// WithMinVersion sets the minimum version constraint for upgrades.
	// Accepts semver constraint syntax (e.g., ">= 1.0.0", "~> 1.2", "^1.0").
	// Versions that don't satisfy the constraint will be filtered out.
	WithMinVersion(constraint string) UpgraderBuilder
	// WithProjectID sets the GitLab project ID.
	WithProjectID(id int) UpgraderBuilder
	// WithBinaryName sets the name of the binary (e.g., "stdb-gen").
	WithBinaryName(name string) UpgraderBuilder
	// WithBinaryPath sets the path to the current binary.
	WithBinaryPath(path string) UpgraderBuilder
	// WithStdout sets the writer for progress output.
	WithStdout(w io.Writer) UpgraderBuilder
	// WithMaxVersions sets the maximum number of versions to fetch.
	WithMaxVersions(n int) UpgraderBuilder
	// WithGitLabToken sets the GitLab API token for authenticated requests.
	WithGitLabToken(token string) UpgraderBuilder
	// Build creates the Upgrader instance.
	Build(ctx context.Context) (Upgrader, error)
}

// Upgrader provides methods for checking and performing upgrades.
type Upgrader interface {
	// ListVersions returns available versions from the repository.
	ListVersions(ctx context.Context) ([]Release, error)
	// GetLatestVersion returns the latest available release.
	GetLatestVersion(ctx context.Context) (Release, error)
	// GetVersion returns a specific version release.
	GetVersion(ctx context.Context, version string) (Release, error)
	// Upgrade performs the upgrade to the specified version.
	// If version is empty, upgrades to the latest version.
	Upgrade(ctx context.Context, version string) (UpgradeResult, error)
	// CurrentVersion returns the current running version.
	CurrentVersion() string
}

// Release represents a release in the repository.
type Release interface {
	// Version returns the semantic version string (e.g., "v1.2.3").
	Version() string
	// TagName returns the git tag name.
	TagName() string
	// AssetURL returns the download URL for the current platform's binary.
	AssetURL() string
	// HasAsset returns true if there is an asset for the current platform.
	HasAsset() bool
	// ReleaseNotes returns the release notes/changelog.
	ReleaseNotes() string
	// PublishedAt returns the release publication time as a string.
	PublishedAt() string
}

// UpgradeResult contains the result of an upgrade operation.
type UpgradeResult interface {
	// FromVersion returns the version before the upgrade.
	FromVersion() string
	// ToVersion returns the version after the upgrade.
	ToVersion() string
	// Success returns true if the upgrade was successful.
	Success() bool
	// Message returns a human-readable result message.
	Message() string
}

// NewUpgraderBuilder creates a new UpgraderBuilder with sensible defaults.
func NewUpgraderBuilder() UpgraderBuilder {
	return &upgraderBuilder{
		maxVersions: 50,
		binaryName:  "stdb-gen",
	}
}
