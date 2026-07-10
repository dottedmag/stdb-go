package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillsCmd_InstallsAllSkills(t *testing.T) {
	t.Parallel()

	out := t.TempDir()

	cmd := newSkillsCmd()
	cmd.SetArgs([]string{"--out", out})
	require.NoError(t, cmd.Execute())

	for _, skill := range []string{"stdb-go-cli", "stdb-go-server", "stdb-go-client"} {
		assert.FileExists(t, filepath.Join(out, skill, "SKILL.md"))
	}
	assert.FileExists(t, filepath.Join(out, "stdb-go-server", "references", "server-reference.md"))
	assert.FileExists(t, filepath.Join(out, "stdb-go-client", "references", "client-reference.md"))
}

func TestSkillsCmd_OutRequired(t *testing.T) {
	t.Parallel()

	cmd := newSkillsCmd()
	cmd.SetArgs([]string{})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out")
}
