package scaffold_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/scaffold"
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
	assert.Contains(t, string(mainGo), "go:generate go run go.digitalxero.dev/stdb-go")
	assert.Contains(t, string(mainGo), "//stdb:init")

	// Check types.go contents
	typesGo, err := os.ReadFile(filepath.Join(outDir, "types.go"))
	require.NoError(t, err)
	assert.Contains(t, string(typesGo), "//stdb:table name=user access=public")
	assert.Contains(t, string(typesGo), "type User struct")

	// Check reducers.go contents
	reducersGo, err := os.ReadFile(filepath.Join(outDir, "reducers.go"))
	require.NoError(t, err)
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
		Build()
	require.NoError(t, err)

	err = s.Generate()
	require.NoError(t, err)

	gomod, err := os.ReadFile(filepath.Join(outDir, "go.mod"))
	require.NoError(t, err)
	assert.Contains(t, string(gomod), "module myproject")
}
