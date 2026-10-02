package parser_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/dottedmag/stdb-go/internal/parser"
)

// fieldByName returns the parsed field with the given Go name.
func fieldByName(t *testing.T, fields []parser.ParsedField, name string) parser.ParsedField {
	t.Helper()
	for _, f := range fields {
		if f.GoName == name {
			return f
		}
	}
	t.Fatalf("field %q not found", name)
	return parser.ParsedField{}
}

func TestParseDefaultTag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "types.go", `package main

//stdb:table name=widget access=public
type Widget struct {
	Id    uint64 `+"`"+`stdb:"primarykey,autoinc"`+"`"+`
	Count uint32 `+"`"+`stdb:"default=5"`+"`"+`
	Name  string `+"`"+`stdb:"index=btree,default='Unknown'"`+"`"+`
	Note  string `+"`"+`stdb:"default=''"`+"`"+`
	Tags  string `+"`"+`stdb:"default='a, b, c'"`+"`"+`
	Quote string `+"`"+`stdb:"default='it\'s'"`+"`"+`
	Blob  []byte `+"`"+`stdb:"default=raw:0xDEADBEEF"`+"`"+`
	Plain uint8  `+"`"+`stdb:"index=direct"`+"`"+`
}
`)
	parsed, err := parser.ParseDirectory(dir)
	require.NoError(t, err)
	require.Len(t, parsed.Tables, 1)
	fields := parsed.Tables[0].Fields

	// No default -> nil pointer.
	assert.Nil(t, fieldByName(t, fields, "Id").Default)
	assert.Nil(t, fieldByName(t, fields, "Plain").Default)

	cases := map[string]string{
		"Count": "5",
		"Name":  "Unknown", // quotes stripped
		"Note":  "",        // empty string, distinct from nil
		"Tags":  "a, b, c", // comma inside quotes preserved
		"Quote": "it's",    // escaped quote unescaped
		"Blob":  "raw:0xDEADBEEF",
	}
	for name, want := range cases {
		f := fieldByName(t, fields, name)
		require.NotNilf(t, f.Default, "field %s should have a default", name)
		assert.Equalf(t, want, *f.Default, "field %s default", name)
	}

	// Other tag options still parse alongside default.
	assert.True(t, fieldByName(t, fields, "Name").IndexBTree)
	assert.True(t, fieldByName(t, fields, "Id").AutoInc)
}
