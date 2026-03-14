package upgrade_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/upgrade"
)

func TestNewUpgraderBuilder(t *testing.T) {
	t.Parallel()

	builder := upgrade.NewUpgraderBuilder()
	require.NotNil(t, builder, "NewUpgraderBuilder should return a non-nil builder")
}

func TestUpgraderBuilder_MethodChaining(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	builder := upgrade.NewUpgraderBuilder().
		WithCurrentVersion("v1.0.0").
		WithMinVersion(">= 0.1.0").
		WithProjectID(12345).
		WithBinaryName("test-binary").
		WithBinaryPath("/usr/local/bin/test-binary").
		WithStdout(&buf).
		WithMaxVersions(10).
		WithGitLabToken("test-token")

	require.NotNil(t, builder, "builder should not be nil after chaining")
}

func TestUpgraderBuilder_Build_InvalidCurrentVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, err := upgrade.NewUpgraderBuilder().
		WithCurrentVersion("not-a-valid-semver").
		Build(ctx)

	require.Error(t, err, "Build should fail with invalid current version")
	assert.True(t, errors.Is(err, upgrade.ErrInvalidVersion), "error should wrap ErrInvalidVersion")
}

func TestUpgraderBuilder_Build_InvalidMinVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, err := upgrade.NewUpgraderBuilder().
		WithCurrentVersion("v1.0.0").
		WithMinVersion("invalid-version").
		Build(ctx)

	require.Error(t, err, "Build should fail with invalid min version")
	assert.True(t, errors.Is(err, upgrade.ErrInvalidVersion), "error should wrap ErrInvalidVersion")
}

func TestUpgraderBuilder_DoubleBuild(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	builder := upgrade.NewUpgraderBuilder().
		WithCurrentVersion("v1.0.0")

	// First build should succeed
	upgrader, err := builder.Build(ctx)
	require.NoError(t, err, "first Build should succeed")
	require.NotNil(t, upgrader, "upgrader should not be nil")

	// Second build should fail
	_, err = builder.Build(ctx)
	require.Error(t, err, "second Build should fail")
	assert.True(t, errors.Is(err, upgrade.ErrAlreadyBuilt), "error should be ErrAlreadyBuilt")
}

func TestUpgraderBuilder_Build_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	upgrader, err := upgrade.NewUpgraderBuilder().
		WithCurrentVersion("v1.0.0").
		WithMinVersion(">= 0.1.0").
		WithProjectID(12345).
		Build(ctx)

	require.NoError(t, err, "Build should succeed with valid configuration")
	require.NotNil(t, upgrader, "upgrader should not be nil")
	assert.Equal(t, "v1.0.0", upgrader.CurrentVersion(), "CurrentVersion should return the configured version")
}

func TestUpgraderBuilder_IgnoresAfterBuild(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	builder := upgrade.NewUpgraderBuilder().
		WithCurrentVersion("v1.0.0")

	upgrader, err := builder.Build(ctx)
	require.NoError(t, err, "Build should succeed")

	// Calls after Build should be ignored
	builder.WithCurrentVersion("v2.0.0")

	assert.Equal(t, "v1.0.0", upgrader.CurrentVersion(), "version should not change after Build")
}

func TestUpgradeError(t *testing.T) {
	t.Parallel()

	t.Run("with version", func(t *testing.T) {
		t.Parallel()
		err := upgrade.NewUpgradeError("download", "v1.2.3", upgrade.ErrDownloadFailed)
		assert.Contains(t, err.Error(), "download")
		assert.Contains(t, err.Error(), "v1.2.3")
		assert.True(t, errors.Is(err, upgrade.ErrDownloadFailed))
	})

	t.Run("without version", func(t *testing.T) {
		t.Parallel()
		err := upgrade.NewUpgradeError("validate", "", upgrade.ErrInvalidVersion)
		assert.Contains(t, err.Error(), "validate")
		assert.NotContains(t, err.Error(), "version v")
		assert.True(t, errors.Is(err, upgrade.ErrInvalidVersion))
	})

	t.Run("unwrap", func(t *testing.T) {
		t.Parallel()
		underlyingErr := errors.New("underlying error")
		err := upgrade.NewUpgradeError("test", "v1.0.0", underlyingErr)
		assert.Equal(t, underlyingErr, errors.Unwrap(err))
	})
}

func TestSentinelErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  error
	}{
		{"ErrAlreadyBuilt", upgrade.ErrAlreadyBuilt},
		{"ErrNoReleasesFound", upgrade.ErrNoReleasesFound},
		{"ErrVersionNotFound", upgrade.ErrVersionNotFound},
		{"ErrBinaryNotFound", upgrade.ErrBinaryNotFound},
		{"ErrDownloadFailed", upgrade.ErrDownloadFailed},
		{"ErrPermissionDenied", upgrade.ErrPermissionDenied},
		{"ErrReplaceFailed", upgrade.ErrReplaceFailed},
		{"ErrInvalidVersion", upgrade.ErrInvalidVersion},
		{"ErrVersionBelowMinimum", upgrade.ErrVersionBelowMinimum},
		{"ErrGitLabClientFailed", upgrade.ErrGitLabClientFailed},
		{"ErrNoBinaryPath", upgrade.ErrNoBinaryPath},
		{"ErrUpgradeInProgress", upgrade.ErrUpgradeInProgress},
		{"ErrSameVersion", upgrade.ErrSameVersion},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.NotNil(t, tc.err, "%s should not be nil", tc.name)
			assert.NotEmpty(t, tc.err.Error(), "%s should have an error message", tc.name)
		})
	}
}

func TestUpgrader_CurrentVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		version string
	}{
		{"standard version", "v1.2.3"},
		{"prerelease version", "v1.0.0-beta.1"},
		{"version with metadata", "v1.0.0+build123"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			upgrader, err := upgrade.NewUpgraderBuilder().
				WithCurrentVersion(tc.version).
				Build(ctx)

			require.NoError(t, err)
			assert.Equal(t, tc.version, upgrader.CurrentVersion())
		})
	}
}

func TestUpgrader_BuildWithEmptyVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	upgrader, err := upgrade.NewUpgraderBuilder().
		Build(ctx)

	require.NoError(t, err, "Build should succeed without version")
	assert.Empty(t, upgrader.CurrentVersion(), "CurrentVersion should be empty when not set")
}

func TestUpgradeError_Is(t *testing.T) {
	t.Parallel()

	baseErr := upgrade.ErrDownloadFailed
	wrappedErr := upgrade.NewUpgradeError("download", "v1.0.0", baseErr)

	assert.True(t, errors.Is(wrappedErr, baseErr), "errors.Is should match the underlying error")
	assert.False(t, errors.Is(wrappedErr, upgrade.ErrReplaceFailed), "errors.Is should not match different error")
}

func TestUpgraderBuilder_MinVersionConstraints(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		constraint string
		wantErr    bool
	}{
		{"simple >=", ">= 1.0.0", false},
		{"simple >", "> 1.0.0", false},
		{"simple <=", "<= 2.0.0", false},
		{"simple <", "< 2.0.0", false},
		{"exact =", "= 1.2.3", false},
		{"tilde ~>", "~> 1.2", false},
		{"caret ^", "^1.0", false},
		{"range", ">= 1.0.0, < 2.0.0", false},
		{"wildcard", "1.x", false},
		{"invalid constraint", "not-a-constraint", true},
		{"invalid chars", ">>= 1.0.0", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			_, err := upgrade.NewUpgraderBuilder().
				WithCurrentVersion("v1.0.0").
				WithMinVersion(tc.constraint).
				WithProjectID(12345).
				Build(ctx)

			if tc.wantErr {
				require.Error(t, err, "Build should fail with invalid constraint: %s", tc.constraint)
				assert.True(t, errors.Is(err, upgrade.ErrInvalidVersion), "error should wrap ErrInvalidVersion")
			} else {
				require.NoError(t, err, "Build should succeed with valid constraint: %s", tc.constraint)
			}
		})
	}
}

func TestUpgraderBuilder_DefaultMinVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// Build without setting minVersion should use the default ">= 0.1.0"
	upgrader, err := upgrade.NewUpgraderBuilder().
		WithCurrentVersion("v1.5.0").
		WithProjectID(12345).
		Build(ctx)

	require.NoError(t, err, "Build should succeed with default minVersion")
	require.NotNil(t, upgrader, "upgrader should not be nil")
}
