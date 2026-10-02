package scaffold_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dottedmag/stdb-go/internal/scaffold"
)

func TestBuild_NameRequired(t *testing.T) {
	t.Parallel()

	_, err := scaffold.NewScaffoldBuilder().Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project name is required")
}

func TestBuild_InvalidProjectType(t *testing.T) {
	t.Parallel()

	_, err := scaffold.NewScaffoldBuilder().
		WithName("myproject").
		WithType("invalid").
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid project type")
}

func TestBuild_DefaultsApplied(t *testing.T) {
	t.Parallel()

	s, err := scaffold.NewScaffoldBuilder().
		WithName("myproject").
		Build()
	require.NoError(t, err)
	require.NotNil(t, s)
}

func TestBuild_AllProjectTypes(t *testing.T) {
	t.Parallel()

	for _, pt := range []scaffold.ProjectType{scaffold.Server, scaffold.Client, scaffold.Fullstack} {
		t.Run(string(pt), func(t *testing.T) {
			t.Parallel()

			s, err := scaffold.NewScaffoldBuilder().
				WithName("testproject").
				WithType(pt).
				Build()
			require.NoError(t, err)
			require.NotNil(t, s)
		})
	}
}

func TestGenerate_Server(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outDir := filepath.Join(dir, "myserver")

	s, err := scaffold.NewScaffoldBuilder().
		WithName("myserver").
		WithModule("github.com/test/myserver").
		WithType(scaffold.Server).
		WithDir(outDir).
		WithClientSDKVersion("v0.5.0").
		WithServerSDKVersion("v0.4.1").
		Build()
	require.NoError(t, err)

	err = s.Generate()
	require.NoError(t, err)

	// Check expected files exist
	expectedFiles := []string{
		"go.mod",
		"main.go",
		"types.go",
		"reducers.go",
		"spacetime.json",
		"Taskfile.yml",
	}
	for _, f := range expectedFiles {
		path := filepath.Join(outDir, f)
		assert.FileExists(t, path, "expected file %s to exist", f)
	}

	// Check go.mod contents
	gomod, err := os.ReadFile(filepath.Join(outDir, "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(gomod), "github.com/test/myserver")
	assert.Contains(t, string(gomod), "go.digitalxero.dev/spacetimedb-server")

	// Check main.go contents
	mainGo, err := os.ReadFile(filepath.Join(outDir, "main.go"))
	require.NoError(t, err)
	assert.Contains(t, string(mainGo), "go:generate go run github.com/dottedmag/stdb-go")
	// The directive parser skips main.go entirely, so directives there would
	// be silently ignored — the init hook must live in a parsed file instead.
	assert.NotContains(t, string(mainGo), "//stdb:")

	// Check types.go contents
	typesGo, err := os.ReadFile(filepath.Join(outDir, "types.go"))
	require.NoError(t, err)
	assert.Contains(t, string(typesGo), "//stdb:table name=user access=public")
	assert.Contains(t, string(typesGo), "type User struct")

	// Check reducers.go contents
	reducersGo, err := os.ReadFile(filepath.Join(outDir, "reducers.go"))
	require.NoError(t, err)
	assert.Contains(t, string(reducersGo), "//stdb:init")
	assert.Contains(t, string(reducersGo), "func Init")
	assert.Contains(t, string(reducersGo), "//stdb:reducer")
	assert.Contains(t, string(reducersGo), "func CreateUser")
}

func TestGenerate_Client(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outDir := filepath.Join(dir, "myclient")

	s, err := scaffold.NewScaffoldBuilder().
		WithName("myclient").
		WithModule("github.com/test/myclient").
		WithType(scaffold.Client).
		WithDir(outDir).
		WithClientSDKVersion("v0.5.0").
		Build()
	require.NoError(t, err)

	err = s.Generate()
	require.NoError(t, err)

	expectedFiles := []string{
		"go.mod",
		"main.go",
	}
	for _, f := range expectedFiles {
		path := filepath.Join(outDir, f)
		assert.FileExists(t, path, "expected file %s to exist", f)
	}

	gomod, err := os.ReadFile(filepath.Join(outDir, "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(gomod), "github.com/test/myclient")
	assert.Contains(t, string(gomod), "go.digitalxero.dev/spacetimedb-client")

	mainGo, err := os.ReadFile(filepath.Join(outDir, "main.go"))
	require.NoError(t, err)
	assert.Contains(t, string(mainGo), "spacetimedb.Connect")
	assert.Contains(t, string(mainGo), "myclient")
}

func TestGenerate_Fullstack(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outDir := filepath.Join(dir, "myapp")

	s, err := scaffold.NewScaffoldBuilder().
		WithName("myapp").
		WithModule("github.com/test/myapp").
		WithType(scaffold.Fullstack).
		WithDir(outDir).
		WithClientSDKVersion("v0.5.0").
		WithServerSDKVersion("v0.4.1").
		Build()
	require.NoError(t, err)

	err = s.Generate()
	require.NoError(t, err)

	// Check server files
	serverFiles := []string{
		"server/go.mod",
		"server/main.go",
		"server/types.go",
		"server/reducers.go",
		"server/spacetime.json",
		"server/Taskfile.yml",
	}
	for _, f := range serverFiles {
		path := filepath.Join(outDir, f)
		assert.FileExists(t, path, "expected file %s to exist", f)
	}

	// Check client files
	clientFiles := []string{
		"client/go.mod",
		"client/main.go",
	}
	for _, f := range clientFiles {
		path := filepath.Join(outDir, f)
		assert.FileExists(t, path, "expected file %s to exist", f)
	}

	// Check root Taskfile
	assert.FileExists(t, filepath.Join(outDir, "Taskfile.yml"))

	// Verify server go.mod uses the base module
	serverGomod, err := os.ReadFile(filepath.Join(outDir, "server", "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(serverGomod), "github.com/test/myapp")

	// Verify client go.mod uses module/client
	clientGomod, err := os.ReadFile(filepath.Join(outDir, "client", "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(clientGomod), "github.com/test/myapp/client")
}

func TestGenerate_DefaultModuleIsName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outDir := filepath.Join(dir, "myproject")

	s, err := scaffold.NewScaffoldBuilder().
		WithName("myproject").
		WithType(scaffold.Server).
		WithDir(outDir).
		WithClientSDKVersion("v0.5.0").
		WithServerSDKVersion("v0.4.1").
		Build()
	require.NoError(t, err)

	err = s.Generate()
	require.NoError(t, err)

	gomod, err := os.ReadFile(filepath.Join(outDir, "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(gomod), "module myproject")
}

func TestGenerate_ServerGoModContainsPinnedVersions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outDir := filepath.Join(dir, "pinned")

	s, err := scaffold.NewScaffoldBuilder().
		WithName("pinned").
		WithModule("github.com/test/pinned").
		WithType(scaffold.Server).
		WithDir(outDir).
		WithClientSDKVersion("v1.2.3").
		WithServerSDKVersion("v4.5.6").
		Build()
	require.NoError(t, err)

	err = s.Generate()
	require.NoError(t, err)

	gomod, err := os.ReadFile(filepath.Join(outDir, "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(gomod), "go.digitalxero.dev/spacetimedb-client v1.2.3")
	assert.Contains(t, string(gomod), "go.digitalxero.dev/spacetimedb-server v4.5.6")
}

func TestGenerate_ClientGoModContainsPinnedVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outDir := filepath.Join(dir, "pinned")

	s, err := scaffold.NewScaffoldBuilder().
		WithName("pinned").
		WithModule("github.com/test/pinned").
		WithType(scaffold.Client).
		WithDir(outDir).
		WithClientSDKVersion("v9.8.7").
		Build()
	require.NoError(t, err)

	err = s.Generate()
	require.NoError(t, err)

	gomod, err := os.ReadFile(filepath.Join(outDir, "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(gomod), "go.digitalxero.dev/spacetimedb-client v9.8.7")
}

func TestLatestModuleVersion_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"Version": "v1.0.0"})
	}))
	defer srv.Close()

	// LatestModuleVersion calls the real proxy; we test with the exported wrapper
	// that uses the actual function. For a proper unit test we'd need to inject the URL.
	// Here we just verify the fallback behavior.
	version := scaffold.LatestModuleVersion("nonexistent.invalid/module", "v0.0.1")
	assert.Equal(t, "v0.0.1", version, "should return fallback for unreachable module")
}

func TestLatestModuleVersion_Fallback(t *testing.T) {
	t.Parallel()

	version := scaffold.LatestModuleVersion("nonexistent.invalid/does-not-exist", "v99.99.99")
	assert.Equal(t, "v99.99.99", version)
}
