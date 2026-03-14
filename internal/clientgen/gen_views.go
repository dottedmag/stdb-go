package clientgen

import (
	"fmt"
	"strings"
)

// generateViews generates view type aliases and registration.
func generateViews(schema *ModuleSchema, pkgName string) ([]byte, error) {
	if len(schema.Views) == 0 {
		return nil, nil
	}

	var w strings.Builder
	w.WriteString(clientFileHeader(pkgName))

	imports := map[string]string{
		"cache": "go.digitalxero.dev/spacetimedb-client/client/cache",
		"bsatn": "go.digitalxero.dev/spacetimedb-client/bsatn",
	}

	var viewCode strings.Builder

	for _, view := range schema.Views {
		goName := toGoName(view.Name)
		defName := toLowerCamel(view.Name) + "ViewDef"

		// View definition struct (same pattern as table defs)
		fmt.Fprintf(&viewCode, "// %s implements cache.TypedTableDef for the %s view.\n", defName, view.Name)
		fmt.Fprintf(&viewCode, "type %s struct{}\n\n", defName)

		// TableName (views use their name in the cache just like tables)
		fmt.Fprintf(&viewCode, "func (%s) TableName() string { return %q }\n\n", defName, view.Name)

		// DecodeRow - views return rows that are decoded the same way
		fmt.Fprintf(&viewCode, "func (%s) DecodeRow(r bsatn.Reader) (*%s, error) {\n", defName, goName)
		fmt.Fprintf(&viewCode, "\treturn Read%s(r)\n", goName)
		fmt.Fprintf(&viewCode, "}\n\n")

		// EncodeRow
		fmt.Fprintf(&viewCode, "func (%s) EncodeRow(row *%s) []byte {\n", defName, goName)
		fmt.Fprintf(&viewCode, "\tw := bsatn.NewWriter(64)\n")
		fmt.Fprintf(&viewCode, "\trow.WriteBsatn(w)\n")
		fmt.Fprintf(&viewCode, "\treturn w.Bytes()\n")
		fmt.Fprintf(&viewCode, "}\n\n")

		// Type alias for typed view cache
		fmt.Fprintf(&viewCode, "// %sView is a type-safe view cache for %s rows.\n", goName, view.Name)
		fmt.Fprintf(&viewCode, "type %sView = cache.TypedTableCache[*%s]\n\n", goName, goName)
	}

	w.WriteString(clientImportBlock(imports))
	w.WriteString(viewCode.String())

	return []byte(w.String()), nil
}
