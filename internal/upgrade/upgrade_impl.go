package upgrade

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Masterminds/semver/v3"
	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// Compile-time interface assertions.
var (
	_ UpgraderBuilder = (*upgraderBuilder)(nil)
	_ Upgrader        = (*upgraderBuilder)(nil)
	_ Release         = (*release)(nil)
	_ UpgradeResult   = (*upgradeResult)(nil)
)

// Default GitLab project ID for stdb-gen.
const defaultProjectID = 79925889

// Default minimum version constraint.
const defaultMinVersion = ">= 0.1.0"

// upgraderBuilder implements both UpgraderBuilder and Upgrader interfaces.
type upgraderBuilder struct {
	// Configuration
	currentVersion string
	minVersion     string
	projectID      int
	binaryName     string
	binaryPath     string
	stdout         io.Writer
	maxVersions    int
	gitlabToken    string

	// Runtime state
	client        *gitlab.Client
	platform      string
	minConstraint *semver.Constraints
	currSemver    *semver.Version
	built         atomic.Bool
	upgrading     atomic.Bool
}

// release implements the Release interface.
type release struct {
	version      string
	tagName      string
	assetURL     string
	hasAsset     bool
	releaseNotes string
	publishedAt  string
}

func (r *release) Version() string      { return r.version }
func (r *release) TagName() string      { return r.tagName }
func (r *release) AssetURL() string     { return r.assetURL }
func (r *release) HasAsset() bool       { return r.hasAsset }
func (r *release) ReleaseNotes() string { return r.releaseNotes }
func (r *release) PublishedAt() string  { return r.publishedAt }

// upgradeResult implements the UpgradeResult interface.
type upgradeResult struct {
	fromVersion string
	toVersion   string
	success     bool
	message     string
}

func (r *upgradeResult) FromVersion() string { return r.fromVersion }
func (r *upgradeResult) ToVersion() string   { return r.toVersion }
func (r *upgradeResult) Success() bool       { return r.success }
func (r *upgradeResult) Message() string     { return r.message }

// Builder methods

func (u *upgraderBuilder) WithCurrentVersion(version string) UpgraderBuilder {
	if u.built.Load() {
		return u
	}
	u.currentVersion = version
	return u
}

func (u *upgraderBuilder) WithMinVersion(version string) UpgraderBuilder {
	if u.built.Load() {
		return u
	}
	u.minVersion = version
	return u
}

func (u *upgraderBuilder) WithProjectID(id int) UpgraderBuilder {
	if u.built.Load() {
		return u
	}
	u.projectID = id
	return u
}

func (u *upgraderBuilder) WithBinaryName(name string) UpgraderBuilder {
	if u.built.Load() {
		return u
	}
	u.binaryName = name
	return u
}

func (u *upgraderBuilder) WithBinaryPath(path string) UpgraderBuilder {
	if u.built.Load() {
		return u
	}
	u.binaryPath = path
	return u
}

func (u *upgraderBuilder) WithStdout(w io.Writer) UpgraderBuilder {
	if u.built.Load() {
		return u
	}
	u.stdout = w
	return u
}

func (u *upgraderBuilder) WithMaxVersions(n int) UpgraderBuilder {
	if u.built.Load() {
		return u
	}
	u.maxVersions = n
	return u
}

func (u *upgraderBuilder) WithGitLabToken(token string) UpgraderBuilder {
	if u.built.Load() {
		return u
	}
	u.gitlabToken = token
	return u
}

func (u *upgraderBuilder) Build(ctx context.Context) (Upgrader, error) {
	if !u.built.CompareAndSwap(false, true) {
		return nil, ErrAlreadyBuilt
	}

	var err error

	// Set defaults
	if u.projectID == 0 {
		u.projectID = defaultProjectID
	}
	if u.binaryName == "" {
		u.binaryName = "stdb-gen"
	}
	if u.stdout == nil {
		u.stdout = os.Stdout
	}
	if u.maxVersions <= 0 {
		u.maxVersions = 50
	}

	// Determine platform
	u.platform = platformKey()

	// Parse current version if provided
	if u.currentVersion != "" {
		u.currSemver, err = semver.NewVersion(u.currentVersion)
		if err != nil {
			return nil, NewUpgradeError("parse_version", u.currentVersion, ErrInvalidVersion)
		}
	}

	// Set default minimum version if not provided
	if u.minVersion == "" {
		u.minVersion = defaultMinVersion
	}

	// Parse minimum version constraint
	u.minConstraint, err = semver.NewConstraint(u.minVersion)
	if err != nil {
		return nil, NewUpgradeError("parse_min_version", u.minVersion, ErrInvalidVersion)
	}

	// Try to determine binary path if not provided
	if u.binaryPath == "" {
		var execPath string
		execPath, err = os.Executable()
		if err != nil {
			return nil, NewUpgradeError("get_executable", "", ErrNoBinaryPath)
		}
		// Resolve any symlinks
		u.binaryPath, err = filepath.EvalSymlinks(execPath)
		if err != nil {
			u.binaryPath = execPath
		}
	}

	// Create GitLab client
	if u.gitlabToken != "" {
		u.client, err = gitlab.NewClient(u.gitlabToken)
	} else {
		u.client, err = gitlab.NewClient("")
	}
	if err != nil {
		return nil, NewUpgradeError("create_client", "", ErrGitLabClientFailed)
	}

	return u, nil
}

// Upgrader methods

func (u *upgraderBuilder) CurrentVersion() string {
	return u.currentVersion
}

func (u *upgraderBuilder) ListVersions(ctx context.Context) ([]Release, error) {
	releases, _, err := u.client.Releases.ListReleases(u.projectID, &gitlab.ListReleasesOptions{
		ListOptions: gitlab.ListOptions{
			PerPage: int64(u.maxVersions),
		},
	})
	if err != nil {
		return nil, NewUpgradeError("list_releases", "", err)
	}

	if len(releases) == 0 {
		return nil, ErrNoReleasesFound
	}

	result := make([]Release, 0, len(releases))
	for _, r := range releases {
		rel := u.convertRelease(r)
		if rel == nil {
			continue
		}

		// Filter out versions that don't satisfy the minimum constraint
		if u.minConstraint != nil {
			v, err := semver.NewVersion(rel.version)
			if err != nil || !u.minConstraint.Check(v) {
				continue
			}
		}

		result = append(result, rel)
	}

	// Sort by version descending
	sort.Slice(result, func(i, j int) bool {
		vi, _ := semver.NewVersion(result[i].Version())
		vj, _ := semver.NewVersion(result[j].Version())
		if vi == nil || vj == nil {
			return result[i].Version() > result[j].Version()
		}
		return vi.GreaterThan(vj)
	})

	return result, nil
}

func (u *upgraderBuilder) GetLatestVersion(ctx context.Context) (Release, error) {
	versions, err := u.ListVersions(ctx)
	if err != nil {
		return nil, err
	}

	// Find the latest version that has an asset for this platform
	for _, v := range versions {
		if v.HasAsset() {
			return v, nil
		}
	}

	return nil, ErrBinaryNotFound
}

func (u *upgraderBuilder) GetVersion(ctx context.Context, version string) (Release, error) {
	// Normalize version
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	// Validate against minimum constraint before fetching
	if u.minConstraint != nil {
		v, err := semver.NewVersion(version)
		if err == nil && !u.minConstraint.Check(v) {
			return nil, NewUpgradeError("validate_version", version, ErrVersionBelowMinimum)
		}
	}

	rel, _, err := u.client.Releases.GetRelease(u.projectID, version)
	if err != nil {
		return nil, NewUpgradeError("get_release", version, ErrVersionNotFound)
	}

	result := u.convertRelease(rel)
	if result == nil {
		return nil, NewUpgradeError("convert_release", version, ErrVersionNotFound)
	}

	return result, nil
}

func (u *upgraderBuilder) Upgrade(ctx context.Context, version string) (UpgradeResult, error) {
	// Ensure only one upgrade at a time
	if !u.upgrading.CompareAndSwap(false, true) {
		return nil, ErrUpgradeInProgress
	}
	defer u.upgrading.Store(false)

	result := &upgradeResult{
		fromVersion: u.currentVersion,
	}

	// Get target release
	var targetRelease Release
	var err error
	if version == "" {
		u.printf("Checking for latest version...\n")
		targetRelease, err = u.GetLatestVersion(ctx)
	} else {
		u.printf("Checking for version %s...\n", version)
		targetRelease, err = u.GetVersion(ctx, version)
	}
	if err != nil {
		result.message = fmt.Sprintf("Failed to get release: %v", err)
		return result, err
	}

	result.toVersion = targetRelease.Version()

	// Check if already at this version
	if u.currSemver != nil {
		targetSemver, err := semver.NewVersion(targetRelease.Version())
		if err == nil && u.currSemver.Equal(targetSemver) {
			result.success = true
			result.message = fmt.Sprintf("Already running version %s", u.currentVersion)
			return result, ErrSameVersion
		}
	}

	// Check minimum version constraint
	if u.minConstraint != nil {
		targetSemver, err := semver.NewVersion(targetRelease.Version())
		if err == nil && !u.minConstraint.Check(targetSemver) {
			result.message = fmt.Sprintf("Version %s does not satisfy minimum constraint %s", targetRelease.Version(), u.minVersion)
			return result, ErrVersionBelowMinimum
		}
	}

	// Check if asset exists for this platform
	if !targetRelease.HasAsset() {
		result.message = fmt.Sprintf("No binary available for platform %s", u.platform)
		return result, ErrBinaryNotFound
	}

	u.printf("Downloading %s...\n", targetRelease.Version())

	// Download the new binary
	newBinaryPath, err := u.downloadBinary(ctx, targetRelease.AssetURL())
	if err != nil {
		result.message = fmt.Sprintf("Download failed: %v", err)
		return result, err
	}
	defer func() { _ = os.Remove(newBinaryPath) }() // Clean up temp file on error

	u.printf("Installing...\n")

	// Replace the current binary
	if err := u.replaceBinary(newBinaryPath); err != nil {
		result.message = fmt.Sprintf("Installation failed: %v", err)
		return result, err
	}

	result.success = true
	result.message = fmt.Sprintf("Successfully upgraded from %s to %s", u.currentVersion, targetRelease.Version())
	u.printf("Done! Upgraded to %s\n", targetRelease.Version())

	return result, nil
}

// Helper methods

func (u *upgraderBuilder) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(u.stdout, format, args...)
}

func (u *upgraderBuilder) convertRelease(r *gitlab.Release) *release {
	if r == nil {
		return nil
	}

	rel := &release{
		version:      r.TagName,
		tagName:      r.TagName,
		releaseNotes: r.Description,
	}

	if r.ReleasedAt != nil {
		rel.publishedAt = r.ReleasedAt.Format(time.RFC3339)
	}

	// Find asset for this platform
	assetName := u.assetName()
	for _, link := range r.Assets.Links {
		if link.Name == assetName || strings.HasSuffix(link.URL, assetName) {
			rel.assetURL = link.URL
			rel.hasAsset = true
			break
		}
	}

	// Also check generic package links
	if !rel.hasAsset && r.Assets.Sources != nil {
		for _, source := range r.Assets.Sources {
			if strings.Contains(source.URL, assetName) {
				rel.assetURL = source.URL
				rel.hasAsset = true
				break
			}
		}
	}

	return rel
}

func (u *upgraderBuilder) assetName() string {
	name := u.binaryName + "-" + u.platform
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func (u *upgraderBuilder) downloadBinary(ctx context.Context, url string) (string, error) {
	// Create temp file
	tmpFile, err := os.CreateTemp("", u.binaryName+"-*")
	if err != nil {
		return "", NewUpgradeError("create_temp", "", err)
	}
	tmpPath := tmpFile.Name()

	// Download
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", NewUpgradeError("create_request", "", err)
	}

	if u.gitlabToken != "" {
		req.Header.Set("PRIVATE-TOKEN", u.gitlabToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", NewUpgradeError("download", "", ErrDownloadFailed)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", NewUpgradeError("download", "", fmt.Errorf("%w: HTTP %d", ErrDownloadFailed, resp.StatusCode))
	}

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", NewUpgradeError("write_temp", "", err)
	}

	_ = tmpFile.Close()

	// Make executable
	if err := os.Chmod(tmpPath, 0755); err != nil {
		_ = os.Remove(tmpPath)
		return "", NewUpgradeError("chmod", "", err)
	}

	return tmpPath, nil
}

func (u *upgraderBuilder) replaceBinary(newPath string) error {
	// Get info about current binary
	info, err := os.Stat(u.binaryPath)
	if err != nil {
		return NewUpgradeError("stat_current", "", err)
	}

	// Create backup path
	backupPath := u.binaryPath + ".old"

	// Remove any existing backup
	_ = os.Remove(backupPath)

	// Rename current binary to backup
	if err := os.Rename(u.binaryPath, backupPath); err != nil {
		// On Windows, the binary might be locked, try copy instead
		if runtime.GOOS == "windows" {
			// For Windows, we'll write to a .new file and the user needs to restart
			newTargetPath := u.binaryPath + ".new"
			if err := copyFile(newPath, newTargetPath, info.Mode()); err != nil {
				return NewUpgradeError("copy_new", "", err)
			}
			u.printf("Note: On Windows, please close this application and rename %s to %s\n", newTargetPath, u.binaryPath)
			return nil
		}
		return NewUpgradeError("backup", "", ErrReplaceFailed)
	}

	// Move new binary to target location
	if err := os.Rename(newPath, u.binaryPath); err != nil {
		// Restore backup on failure
		_ = os.Rename(backupPath, u.binaryPath)
		return NewUpgradeError("install", "", ErrReplaceFailed)
	}

	// Preserve permissions
	if err := os.Chmod(u.binaryPath, info.Mode()); err != nil {
		// Non-fatal, but log it
		u.printf("Warning: Could not preserve file permissions: %v\n", err)
	}

	// Remove backup
	_ = os.Remove(backupPath)

	return nil
}

// platformKey returns the platform identifier (e.g., "linux-amd64", "darwin-arm64").
func platformKey() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// copyFile copies a file preserving permissions.
func copyFile(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer func() { _ = dstFile.Close() }()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
