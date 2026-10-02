package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	agentskills "github.com/dottedmag/stdb-go/agent_skills"
	"github.com/dottedmag/stdb-go/internal/skills"
)

func newSkillsCmd() *cobra.Command {
	var out string

	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Install AI coding-agent skills for stdb-go development",
		Long: `Install bundled AI coding-agent skills into a skills directory.

Extracts three skills: stdb-go-cli (using this CLI), stdb-go-server
(writing SpacetimeDB server modules with spacetimedb-server), and
stdb-go-client (writing Go clients with spacetimedb-client).

Existing files are overwritten, so re-running after a CLI upgrade
refreshes the installed skills.`,
		Example: `  # Install into Claude Code's user skills directory
  stdb-go skills --out ~/.claude/skills

  # Install into a project's skills directory
  stdb-go skills --out .claude/skills`,
		RunE: func(cmd *cobra.Command, args []string) error {
			inst, err := skills.NewInstallerBuilder().
				WithFS(agentskills.FS).
				WithOutDir(out).
				Build()
			if err != nil {
				return err
			}

			written, err := inst.Install()
			if err != nil {
				return err
			}

			counts := make(map[string]int)
			var order []string
			for _, path := range written {
				skill, _, _ := strings.Cut(path, "/")
				if counts[skill] == 0 {
					order = append(order, skill)
				}
				counts[skill]++
			}
			for _, skill := range order {
				noun := "files"
				if counts[skill] == 1 {
					noun = "file"
				}
				fmt.Fprintf(os.Stderr, "skills: installed %s (%d %s)\n", skill, counts[skill], noun)
			}
			fmt.Fprintf(os.Stderr, "skills: %d files written to %s\n", len(written), inst.OutDir())
			return nil
		},
	}

	cmd.Flags().StringVar(&out, "out", "", "target skills directory (e.g. ~/.claude/skills)")
	_ = cmd.MarkFlagRequired("out")

	return cmd
}
