package skills_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/skills"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"stdb-go-cli/SKILL.md":                          {Data: []byte("# cli skill\n")},
		"stdb-go-server/SKILL.md":                       {Data: []byte("# server skill\n")},
		"stdb-go-server/references/server-reference.md": {Data: []byte("# server reference\n")},
	}
}

func TestBuild_FSRequired(t *testing.T) {
	t.Parallel()

	_, err := skills.NewInstallerBuilder().WithOutDir(t.TempDir()).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source filesystem is required")
}

func TestBuild_OutDirRequired(t *testing.T) {
	t.Parallel()

	_, err := skills.NewInstallerBuilder().WithFS(testFS()).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output directory is required")
}

func TestBuild_TildeExpansion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	inst, err := skills.NewInstallerBuilder().
		WithFS(testFS()).
		WithOutDir("~/agent/skills").
		Build()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "agent", "skills"), inst.OutDir())
}

func TestBuild_CleansOutDir(t *testing.T) {
	t.Parallel()

	inst, err := skills.NewInstallerBuilder().
		WithFS(testFS()).
		WithOutDir(filepath.Join(t.TempDir(), "a", "..", "b")).
		Build()
	require.NoError(t, err)
	assert.Equal(t, "b", filepath.Base(inst.OutDir()))
	assert.NotContains(t, inst.OutDir(), "..")
}

func TestInstall_ExtractsNestedFiles(t *testing.T) {
	t.Parallel()

	out := t.TempDir()
	inst, err := skills.NewInstallerBuilder().
		WithFS(testFS()).
		WithOutDir(out).
		Build()
	require.NoError(t, err)

	written, err := inst.Install()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"stdb-go-cli/SKILL.md",
		"stdb-go-server/SKILL.md",
		"stdb-go-server/references/server-reference.md",
	}, written)

	content, err := os.ReadFile(filepath.Join(out, "stdb-go-server", "references", "server-reference.md"))
	require.NoError(t, err)
	assert.Equal(t, "# server reference\n", string(content))

	info, err := os.Stat(filepath.Join(out, "stdb-go-cli", "SKILL.md"))
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestInstall_OverwritesExisting(t *testing.T) {
	t.Parallel()

	out := t.TempDir()
	stale := filepath.Join(out, "stdb-go-cli", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(stale), 0755))
	require.NoError(t, os.WriteFile(stale, []byte("stale content"), 0644))

	inst, err := skills.NewInstallerBuilder().
		WithFS(testFS()).
		WithOutDir(out).
		Build()
	require.NoError(t, err)

	_, err = inst.Install()
	require.NoError(t, err)

	content, err := os.ReadFile(stale)
	require.NoError(t, err)
	assert.Equal(t, "# cli skill\n", string(content))
}

func TestInstall_SkipsGoFiles(t *testing.T) {
	t.Parallel()

	fsys := testFS()
	fsys["embed.go"] = &fstest.MapFile{Data: []byte("package agentskills\n")}

	out := t.TempDir()
	inst, err := skills.NewInstallerBuilder().
		WithFS(fsys).
		WithOutDir(out).
		Build()
	require.NoError(t, err)

	written, err := inst.Install()
	require.NoError(t, err)
	assert.NotContains(t, written, "embed.go")
	assert.NoFileExists(t, filepath.Join(out, "embed.go"))
}

// failFS wraps an fs.FS and fails Open for a single path, exercising the
// installer's read-error handling.
type failFS struct {
	fs.FS
	failPath string
}

func (f failFS) Open(name string) (fs.File, error) {
	if name == f.failPath {
		return nil, errors.New("boom")
	}
	return f.FS.Open(name)
}

func TestInstall_ReadFailure(t *testing.T) {
	t.Parallel()

	inst, err := skills.NewInstallerBuilder().
		WithFS(failFS{FS: testFS(), failPath: "stdb-go-cli/SKILL.md"}).
		WithOutDir(t.TempDir()).
		Build()
	require.NoError(t, err)

	_, err = inst.Install()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skills: reading stdb-go-cli/SKILL.md")
}

func TestInstall_OutDirIsFile(t *testing.T) {
	t.Parallel()

	out := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(out, []byte("file"), 0644))

	inst, err := skills.NewInstallerBuilder().
		WithFS(testFS()).
		WithOutDir(out).
		Build()
	require.NoError(t, err)

	_, err = inst.Install()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "skills: creating directory")
}
