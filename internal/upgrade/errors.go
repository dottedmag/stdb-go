// Package upgrade provides self-upgrade functionality for stdb-gen.
package upgrade

import (
	"errors"
	"fmt"
)

// Sentinel errors for upgrade operations.
var (
	// ErrAlreadyBuilt is returned when Build is called more than once on an upgrader.
	ErrAlreadyBuilt = errors.New("upgrader already built")

	// ErrNoReleasesFound is returned when no releases are found in the repository.
	ErrNoReleasesFound = errors.New("no releases found")

	// ErrVersionNotFound is returned when a specific version is not found.
	ErrVersionNotFound = errors.New("version not found")

	// ErrBinaryNotFound is returned when the binary asset is not found for the platform.
	ErrBinaryNotFound = errors.New("binary not found for platform")

	// ErrDownloadFailed is returned when downloading the binary fails.
	ErrDownloadFailed = errors.New("failed to download binary")

	// ErrPermissionDenied is returned when there are insufficient permissions.
	ErrPermissionDenied = errors.New("permission denied")

	// ErrReplaceFailed is returned when replacing the binary fails.
	ErrReplaceFailed = errors.New("failed to replace binary")

	// ErrInvalidVersion is returned when a version string is invalid.
	ErrInvalidVersion = errors.New("invalid version format")

	// ErrVersionBelowMinimum is returned when attempting to upgrade to a version below minimum.
	ErrVersionBelowMinimum = errors.New("version below minimum supported version")

	// ErrGitLabClientFailed is returned when the GitLab client fails to initialize.
	ErrGitLabClientFailed = errors.New("failed to create GitLab client")

	// ErrNoBinaryPath is returned when the binary path cannot be determined.
	ErrNoBinaryPath = errors.New("cannot determine binary path")

	// ErrUpgradeInProgress is returned when an upgrade is already running.
	ErrUpgradeInProgress = errors.New("upgrade already in progress")

	// ErrSameVersion is returned when attempting to upgrade to the current version.
	ErrSameVersion = errors.New("already running requested version")
)

// UpgradeError provides detailed error information for upgrade operations.
type UpgradeError struct {
	// Op is the operation that failed (e.g., "download", "replace", "validate").
	Op string
	// Version is the version being upgraded to, if applicable.
	Version string
	// Err is the underlying error.
	Err error
}

// Error implements the error interface.
func (e *UpgradeError) Error() string {
	if e.Version != "" {
		return fmt.Sprintf("upgrade %s failed for version %s: %v", e.Op, e.Version, e.Err)
	}
	return fmt.Sprintf("upgrade %s failed: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error.
func (e *UpgradeError) Unwrap() error {
	return e.Err
}

// NewUpgradeError creates a new UpgradeError.
func NewUpgradeError(op, version string, err error) *UpgradeError {
	return &UpgradeError{
		Op:      op,
		Version: version,
		Err:     err,
	}
}

// Is implements errors.Is for UpgradeError.
func (e *UpgradeError) Is(target error) bool {
	return errors.Is(e.Err, target)
}
