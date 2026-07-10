package agentskills_test

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentskills "go.digitalxero.dev/stdb-go/agent_skills"
)

func TestFS_ContainsAllSkillFiles(t *testing.T) {
	t.Parallel()

	expected := []string{
		"stdb-go-cli/SKILL.md",
		"stdb-go-server/SKILL.md",
		"stdb-go-server/references/server-reference.md",
		"stdb-go-client/SKILL.md",
		"stdb-go-client/references/client-reference.md",
	}
	for _, path := range expected {
		data, err := fs.ReadFile(agentskills.FS, path)
		require.NoError(t, err, "expected %s to be embedded", path)
		assert.NotEmpty(t, data, "expected %s to be non-empty", path)
	}
}

func TestFS_SkillFrontmatterNameMatchesDir(t *testing.T) {
	t.Parallel()

	matches, err := fs.Glob(agentskills.FS, "*/SKILL.md")
	require.NoError(t, err)
	require.NotEmpty(t, matches)

	for _, path := range matches {
		dir, _, _ := strings.Cut(path, "/")
		data, err := fs.ReadFile(agentskills.FS, path)
		require.NoError(t, err)
		assert.Contains(t, string(data), "name: "+dir,
			"frontmatter name in %s should match its directory", path)
	}
}

func TestFS_ContainsNoGoFiles(t *testing.T) {
	t.Parallel()

	err := fs.WalkDir(agentskills.FS, ".", func(path string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		assert.False(t, strings.HasSuffix(path, ".go"), "unexpected Go file embedded: %s", path)
		return nil
	})
	require.NoError(t, err)
}
