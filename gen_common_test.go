package main

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"EntityId", "entity_id"},
		{"HTTPServer", "http_server"},
		{"HTMLParser", "html_parser"},
		{"camelCase", "camel_case"},
		{"simple", "simple"},
		{"already_snake", "already_snake"},
		{"ID", "id"},
		{"TestA", "test_a"},
		{"MyHTTPSClient", "my_https_client"},
		{"URL2Handler", "url2_handler"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input+"->"+tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, toSnakeCase(tt.input))
		})
	}
}

func TestToPascalCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"one_u8", "OneU8"},
		{"hello_world", "HelloWorld"},
		{"simple", "Simple"},
		{"", ""},
		{"already_pascal", "AlreadyPascal"},
		{"a_b_c", "ABC"},
		{"_leading", "Leading"},
		{"trailing_", "Trailing"},
	}

	for _, tt := range tests {
		t.Run(tt.input+"->"+tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, toPascalCase(tt.input))
		})
	}
}

func TestFileHeader(t *testing.T) {
	t.Run("contains DO NOT EDIT marker", func(t *testing.T) {
		hdr := fileHeader("mypkg")
		assert.Contains(t, hdr, "DO NOT EDIT")
	})

	t.Run("contains package declaration", func(t *testing.T) {
		hdr := fileHeader("mypkg")
		assert.Contains(t, hdr, "package mypkg")
	})

	t.Run("different package name", func(t *testing.T) {
		hdr := fileHeader("spacetimedb")
		assert.Contains(t, hdr, "package spacetimedb")
		assert.Contains(t, hdr, "DO NOT EDIT")
	})

	t.Run("ends with blank line", func(t *testing.T) {
		hdr := fileHeader("test")
		assert.True(t, strings.HasSuffix(hdr, "\n\n"), "header should end with double newline")
	})
}

func TestImportBlock(t *testing.T) {
	t.Run("empty map returns empty string", func(t *testing.T) {
		result := importBlock(map[string]string{})
		assert.Equal(t, "", result)
	})

	t.Run("nil map returns empty string", func(t *testing.T) {
		result := importBlock(nil)
		assert.Equal(t, "", result)
	})

	t.Run("single import without alias", func(t *testing.T) {
		result := importBlock(map[string]string{
			"fmt": "fmt",
		})
		assert.Contains(t, result, "import (")
		assert.Contains(t, result, `"fmt"`)
		assert.Contains(t, result, ")")
		// Should NOT have an alias prefix since alias matches the default
		assert.NotContains(t, result, `fmt "fmt"`)
	})

	t.Run("import with custom alias", func(t *testing.T) {
		result := importBlock(map[string]string{
			"stdbTypes": "github.com/clockworklabs/SpacetimeDB/sdks/go/types",
		})
		assert.Contains(t, result, `stdbTypes "github.com/clockworklabs/SpacetimeDB/sdks/go/types"`)
	})

	t.Run("import where alias matches default is not aliased", func(t *testing.T) {
		result := importBlock(map[string]string{
			"types": "github.com/clockworklabs/SpacetimeDB/sdks/go/types",
		})
		// alias "types" equals pathToDefaultAlias("github.com/.../types") -> "types"
		// so it should NOT be aliased
		assert.Contains(t, result, `"github.com/clockworklabs/SpacetimeDB/sdks/go/types"`)
		assert.NotContains(t, result, `types "github.com/clockworklabs/SpacetimeDB/sdks/go/types"`)
	})

	t.Run("multiple imports", func(t *testing.T) {
		imports := map[string]string{
			"fmt":     "fmt",
			"strings": "strings",
		}
		result := importBlock(imports)
		assert.Contains(t, result, "import (")
		assert.Contains(t, result, `"fmt"`)
		assert.Contains(t, result, `"strings"`)
	})

	t.Run("empty alias treated as no alias", func(t *testing.T) {
		result := importBlock(map[string]string{
			"": "fmt",
		})
		assert.Contains(t, result, `"fmt"`)
		// Empty alias should not produce a prefix
		assert.NotContains(t, result, ` "fmt"`)
	})
}

func TestPathToDefaultAlias(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"github.com/foo/bar", "bar"},
		{"single", "single"},
		{"github.com/clockworklabs/SpacetimeDB/sdks/go/types", "types"},
		{"fmt", "fmt"},
		{"a/b/c/d", "d"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, pathToDefaultAlias(tt.input))
		})
	}
}

func TestStrPtr(t *testing.T) {
	t.Run("returns pointer to value", func(t *testing.T) {
		p := strPtr("hello")
		require.NotNil(t, p)
		assert.Equal(t, "hello", *p)
	})

	t.Run("empty string", func(t *testing.T) {
		p := strPtr("")
		require.NotNil(t, p)
		assert.Equal(t, "", *p)
	})

	t.Run("different calls return different pointers", func(t *testing.T) {
		p1 := strPtr("same")
		p2 := strPtr("same")
		// They should be different pointer addresses even though values match
		assert.NotSame(t, p1, p2)
		assert.Equal(t, *p1, *p2)
	})
}

func TestIndent(t *testing.T) {
	t.Run("single line", func(t *testing.T) {
		result := indent("hello", "\t")
		assert.Equal(t, "\thello", result)
	})

	t.Run("multiple lines", func(t *testing.T) {
		result := indent("line1\nline2\nline3", "\t")
		assert.Equal(t, "\tline1\n\tline2\n\tline3", result)
	})

	t.Run("empty lines preserved without prefix", func(t *testing.T) {
		result := indent("line1\n\nline3", "\t")
		assert.Equal(t, "\tline1\n\n\tline3", result)
	})

	t.Run("empty string", func(t *testing.T) {
		result := indent("", "\t")
		assert.Equal(t, "", result)
	})

	t.Run("custom prefix", func(t *testing.T) {
		result := indent("a\nb", "  ")
		assert.Equal(t, "  a\n  b", result)
	})

	t.Run("empty prefix is identity", func(t *testing.T) {
		result := indent("foo\nbar", "")
		assert.Equal(t, "foo\nbar", result)
	})
}

func TestQuote(t *testing.T) {
	t.Run("simple string", func(t *testing.T) {
		assert.Equal(t, `"hello"`, quote("hello"))
	})

	t.Run("empty string", func(t *testing.T) {
		assert.Equal(t, `""`, quote(""))
	})

	t.Run("string with double quotes", func(t *testing.T) {
		result := quote(`say "hi"`)
		assert.Contains(t, result, `\"`)
		// Should be a valid Go string literal
		assert.True(t, strings.HasPrefix(result, `"`))
		assert.True(t, strings.HasSuffix(result, `"`))
	})

	t.Run("string with newline", func(t *testing.T) {
		result := quote("line1\nline2")
		assert.Contains(t, result, `\n`)
	})

	t.Run("string with tab", func(t *testing.T) {
		result := quote("col1\tcol2")
		assert.Contains(t, result, `\t`)
	})

	t.Run("string with backslash", func(t *testing.T) {
		result := quote(`path\to\file`)
		assert.Contains(t, result, `\\`)
	})
}

func TestToParamName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"EntityId", "entityId"},
		{"ID", "iD"},
		{"Name", "name"},
		{"", ""},
		{"already", "already"},
		{"X", "x"},
	}

	for _, tt := range tests {
		t.Run(tt.input+"->"+tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, toParamName(tt.input))
		})
	}
}

func TestOptTmpVar(t *testing.T) {
	tests := []struct {
		label    string
		expected string
	}{
		{"Name", "tmpName"},
		{"name_val", "tmpNameVal"},
		{"field", "tmpField"},
		{"nested_option_val", "tmpNestedOptionVal"},
		{"a_b_c", "tmpABC"},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			assert.Equal(t, tt.expected, optTmpVar(tt.label))
		})
	}

	t.Run("different labels produce different names", func(t *testing.T) {
		names := make(map[string]bool)
		labels := []string{"foo", "bar", "baz", "foo_val", "bar_val"}
		for _, label := range labels {
			name := optTmpVar(label)
			assert.False(t, names[name], "duplicate tmp var name: %s from label %s", name, label)
			names[name] = true
		}
	})
}

// TestImportBlockSorting verifies that importBlock output is deterministic
// for a given set of imports (since map iteration order is random, we check
// that all entries are present).
func TestImportBlockAllPresent(t *testing.T) {
	imports := map[string]string{
		"fmt":       "fmt",
		"strings":   "strings",
		"stdbBsatn": "github.com/clockworklabs/SpacetimeDB/sdks/go/bsatn",
	}

	result := importBlock(imports)

	// Extract the import lines.
	lines := strings.Split(result, "\n")
	var importLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && trimmed != "import (" && trimmed != ")" {
			importLines = append(importLines, trimmed)
		}
	}
	sort.Strings(importLines)

	assert.Len(t, importLines, 3)
	// Verify all expected imports are present (order is non-deterministic from map iteration).
	assert.Contains(t, importLines, `"fmt"`)
	assert.Contains(t, importLines, `"strings"`)
	assert.Contains(t, importLines, `stdbBsatn "github.com/clockworklabs/SpacetimeDB/sdks/go/bsatn"`)
}
