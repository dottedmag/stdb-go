// Package agentskills embeds the AI coding-agent skills that
// `stdb-go skills` installs. Each top-level directory is one skill:
// a SKILL.md plus optional references/ files.
package agentskills

import "embed"

// FS holds the bundled skill files. The skill directories are listed
// explicitly (rather than all:*) so this package's own Go sources are
// never embedded.
//
//go:embed all:stdb-go-cli all:stdb-go-server all:stdb-go-client
var FS embed.FS
