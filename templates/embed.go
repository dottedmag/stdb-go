package templates

import "embed"

//go:embed server/*.tmpl client/*.tmpl fullstack/*.tmpl
var FS embed.FS
