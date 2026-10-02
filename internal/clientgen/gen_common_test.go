package clientgen_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dottedmag/stdb-go/internal/clientgen"
)

func TestToGoName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"a", "A"},
		{"player", "Player"},
		{"add_player", "AddPlayer"},
		{"player_id", "PlayerID"},
		{"http_url", "HTTPURL"},
		{"simple", "Simple"},
		{"my_api_key", "MyAPIKey"},
		{"json_data", "JSONData"},
		{"bsatn_reader", "BSATNReader"},
		{"uuid_field", "UUIDField"},
		{"pk_column", "PKColumn"},
		{"db_name", "DBName"},
		{"__leading_underscores", "LeadingUnderscores"},
		{"trailing__", "Trailing"},
		{"double__underscore", "DoubleUnderscore"},
		{"x", "X"},
		{"ALL_CAPS", "ALLCAPS"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := clientgen.ToGoNameForTest(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToLowerCamel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"simple", "simple"},
		{"add_player", "addPlayer"},
		{"player_id", "playerID"},
		{"http_url", "httpurl"},
		{"x", "x"},
		{"ID", "id"},
		{"my_api_key", "myAPIKey"},
		{"a_b", "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := clientgen.ToLowerCamelForTest(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsCommonAcronym(t *testing.T) {
	acronyms := []string{
		"ID", "URL", "URI", "HTTP", "HTTPS", "API", "SQL", "JSON", "XML",
		"HTML", "CSS", "JS", "PK", "FK", "DB", "IP", "TCP", "UDP", "RPC",
		"UUID", "BSATN",
	}
	for _, a := range acronyms {
		t.Run(a+"_true", func(t *testing.T) {
			assert.True(t, clientgen.IsCommonAcronymForTest(a))
		})
	}

	nonAcronyms := []string{"Go", "RUST", "PLAYER", "FOO", "name", "id", "abc"}
	for _, na := range nonAcronyms {
		t.Run(na+"_false", func(t *testing.T) {
			assert.False(t, clientgen.IsCommonAcronymForTest(na))
		})
	}
}

func TestGoTypeForBuiltin(t *testing.T) {
	tests := []struct {
		builtin  clientgen.BuiltinType
		expected string
	}{
		{clientgen.BuiltinBool, "bool"},
		{clientgen.BuiltinU8, "uint8"},
		{clientgen.BuiltinU16, "uint16"},
		{clientgen.BuiltinU32, "uint32"},
		{clientgen.BuiltinU64, "uint64"},
		{clientgen.BuiltinU128, "types.U128"},
		{clientgen.BuiltinU256, "types.U256"},
		{clientgen.BuiltinI8, "int8"},
		{clientgen.BuiltinI16, "int16"},
		{clientgen.BuiltinI32, "int32"},
		{clientgen.BuiltinI64, "int64"},
		{clientgen.BuiltinI128, "types.I128"},
		{clientgen.BuiltinI256, "types.I256"},
		{clientgen.BuiltinF32, "float32"},
		{clientgen.BuiltinF64, "float64"},
		{clientgen.BuiltinString, "string"},
		{clientgen.BuiltinBytes, "[]byte"},
		{clientgen.BuiltinType("UnknownType"), "any"},
	}

	for _, tt := range tests {
		t.Run(string(tt.builtin), func(t *testing.T) {
			result := clientgen.GoTypeForBuiltinForTest(tt.builtin)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGoTypeForAlgebraic(t *testing.T) {
	typespace := []clientgen.AlgebraicType{
		{
			Kind: clientgen.ATKProduct,
			Product: &clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__identity__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU256}},
				},
			},
		},
	}
	types := []clientgen.TypeSchema{
		{Name: "MyType", TypeRef: 0},
	}

	tests := []struct {
		name     string
		at       clientgen.AlgebraicType
		expected string
	}{
		{
			name:     "Builtin",
			at:       clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64},
			expected: "uint64",
		},
		{
			name:     "Ref",
			at:       clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 0},
			expected: "MyType",
		},
		{
			name: "Product_empty",
			at: clientgen.AlgebraicType{
				Kind:    clientgen.ATKProduct,
				Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}},
			},
			expected: "struct{}",
		},
		{
			name: "Product_special",
			at: clientgen.AlgebraicType{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "__identity__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU256}},
					},
				},
			},
			expected: "types.Identity",
		},
		{
			name: "Sum_option",
			at: clientgen.AlgebraicType{
				Kind: clientgen.ATKSum,
				Sum: &clientgen.SumType{
					Variants: []clientgen.SumTypeVariant{
						{Name: "some", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
						{Name: "none", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{}}},
					},
				},
			},
			expected: "*string",
		},
		{
			name: "Sum_ScheduleAt",
			at: clientgen.AlgebraicType{
				Kind: clientgen.ATKSum,
				Sum: &clientgen.SumType{
					Variants: []clientgen.SumTypeVariant{
						{Name: "Interval", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
						{Name: "Time", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
					},
				},
			},
			expected: "types.ScheduleAt",
		},
		{
			name: "Array_U8",
			at: clientgen.AlgebraicType{
				Kind:    clientgen.ATKArray,
				ArrayTy: &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU8},
			},
			expected: "[]byte",
		},
		{
			name: "Array_String",
			at: clientgen.AlgebraicType{
				Kind:    clientgen.ATKArray,
				ArrayTy: &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString},
			},
			expected: "[]string",
		},
		{
			name: "Map",
			at: clientgen.AlgebraicType{
				Kind:     clientgen.ATKMap,
				MapKey:   &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString},
				MapValue: &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU32},
			},
			expected: "map[string]uint32",
		},
		{
			name:     "Unknown",
			at:       clientgen.AlgebraicType{Kind: 99},
			expected: "any",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clientgen.GoTypeForAlgebraicForTest(&tt.at, typespace, types)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGoTypeForRef(t *testing.T) {
	typespace := []clientgen.AlgebraicType{
		// 0: a product type (Identity special type)
		{
			Kind: clientgen.ATKProduct,
			Product: &clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__identity__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU256}},
				},
			},
		},
		// 1: a regular product type
		{
			Kind: clientgen.ATKProduct,
			Product: &clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "x", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU32}},
				},
			},
		},
	}
	types := []clientgen.TypeSchema{
		{Name: "Player", TypeRef: 1},
	}

	t.Run("named_type_lookup", func(t *testing.T) {
		result := clientgen.GoTypeForRefForTest(1, typespace, types)
		assert.Equal(t, "Player", result)
	})

	t.Run("special_type_fallback", func(t *testing.T) {
		result := clientgen.GoTypeForRefForTest(0, typespace, types)
		assert.Equal(t, "types.Identity", result)
	})

	t.Run("out_of_bounds", func(t *testing.T) {
		result := clientgen.GoTypeForRefForTest(99, typespace, types)
		assert.Equal(t, "any", result)
	})

	t.Run("negative_ref", func(t *testing.T) {
		result := clientgen.GoTypeForRefForTest(-1, typespace, types)
		assert.Equal(t, "any", result)
	})
}

func TestGoTypeForSum(t *testing.T) {
	typespace := []clientgen.AlgebraicType{}
	types := []clientgen.TypeSchema{}

	t.Run("option_lowercase", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "some", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
				{Name: "none", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{}}},
			},
		}
		assert.Equal(t, "*string", clientgen.GoTypeForSumForTest(st, typespace, types))
	})

	t.Run("option_uppercase", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "Some", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
				{Name: "None", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{}}},
			},
		}
		assert.Equal(t, "*uint64", clientgen.GoTypeForSumForTest(st, typespace, types))
	})

	t.Run("schedule_at", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "Interval", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
				{Name: "Time", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
			},
		}
		assert.Equal(t, "types.ScheduleAt", clientgen.GoTypeForSumForTest(st, typespace, types))
	})

	t.Run("non_matching", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "A", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{}}},
				{Name: "B", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{}}},
			},
		}
		assert.Equal(t, "any", clientgen.GoTypeForSumForTest(st, typespace, types))
	})
}

func TestIsOptionType(t *testing.T) {
	t.Run("valid_lowercase", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "some"},
				{Name: "none"},
			},
		}
		assert.True(t, clientgen.IsOptionTypeForTest(st))
	})

	t.Run("valid_uppercase", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "Some"},
				{Name: "None"},
			},
		}
		assert.True(t, clientgen.IsOptionTypeForTest(st))
	})

	t.Run("wrong_names", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "yes"},
				{Name: "no"},
			},
		}
		assert.False(t, clientgen.IsOptionTypeForTest(st))
	})

	t.Run("wrong_count_one", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "some"},
			},
		}
		assert.False(t, clientgen.IsOptionTypeForTest(st))
	})

	t.Run("wrong_count_three", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "some"},
				{Name: "none"},
				{Name: "extra"},
			},
		}
		assert.False(t, clientgen.IsOptionTypeForTest(st))
	})
}

func TestIsScheduleAtType(t *testing.T) {
	t.Run("valid_interval_time", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "Interval"},
				{Name: "Time"},
			},
		}
		assert.True(t, clientgen.IsScheduleAtTypeForTest(st))
	})

	t.Run("valid_time_interval", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "Time"},
				{Name: "Interval"},
			},
		}
		assert.True(t, clientgen.IsScheduleAtTypeForTest(st))
	})

	t.Run("invalid_wrong_names", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "Foo"},
				{Name: "Bar"},
			},
		}
		assert.False(t, clientgen.IsScheduleAtTypeForTest(st))
	})

	t.Run("invalid_wrong_count", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "Interval"},
			},
		}
		assert.False(t, clientgen.IsScheduleAtTypeForTest(st))
	})
}

func TestNeedsTypesImport(t *testing.T) {
	assert.True(t, clientgen.NeedsTypesImportForTest("types.Identity"))
	assert.True(t, clientgen.NeedsTypesImportForTest("types.U128"))
	assert.True(t, clientgen.NeedsTypesImportForTest("types.ScheduleAt"))
	assert.False(t, clientgen.NeedsTypesImportForTest("uint64"))
	assert.False(t, clientgen.NeedsTypesImportForTest("string"))
	assert.False(t, clientgen.NeedsTypesImportForTest("[]byte"))
	assert.False(t, clientgen.NeedsTypesImportForTest("*string"))
}

func TestClientFileHeader(t *testing.T) {
	header := clientgen.ClientFileHeaderForTest("module_bindings")
	assert.Contains(t, header, "Code generated")
	assert.Contains(t, header, "DO NOT EDIT")
	assert.Contains(t, header, "package module_bindings")
}

func TestClientImportBlock(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		result := clientgen.ClientImportBlockForTest(map[string]string{})
		assert.Equal(t, "", result)
	})

	t.Run("single", func(t *testing.T) {
		result := clientgen.ClientImportBlockForTest(map[string]string{
			"fmt": "fmt",
		})
		assert.Contains(t, result, "import (")
		assert.Contains(t, result, `"fmt"`)
	})

	t.Run("multiple_sorted", func(t *testing.T) {
		result := clientgen.ClientImportBlockForTest(map[string]string{
			"bsatn": "go.digitalxero.dev/spacetimedb-client/bsatn",
			"types": "go.digitalxero.dev/spacetimedb-client/types",
		})
		assert.Contains(t, result, "import (")
		assert.Contains(t, result, `"go.digitalxero.dev/spacetimedb-client/bsatn"`)
		assert.Contains(t, result, `"go.digitalxero.dev/spacetimedb-client/types"`)
	})

	t.Run("aliased", func(t *testing.T) {
		result := clientgen.ClientImportBlockForTest(map[string]string{
			"myalias": "some/package/path",
		})
		assert.Contains(t, result, `myalias "some/package/path"`)
	})

	t.Run("default_alias_no_prefix", func(t *testing.T) {
		result := clientgen.ClientImportBlockForTest(map[string]string{
			"bsatn": "go.digitalxero.dev/spacetimedb-client/bsatn",
		})
		// alias matches last path segment, so no prefix needed
		assert.NotContains(t, result, `bsatn "`)
		assert.Contains(t, result, `"go.digitalxero.dev/spacetimedb-client/bsatn"`)
	})
}

func TestPathToAlias(t *testing.T) {
	assert.Equal(t, "bsatn", clientgen.PathToAliasForTest("go.digitalxero.dev/spacetimedb-client/bsatn"))
	assert.Equal(t, "types", clientgen.PathToAliasForTest("go.digitalxero.dev/spacetimedb-client/types"))
	assert.Equal(t, "fmt", clientgen.PathToAliasForTest("fmt"))
	assert.Equal(t, "cache", clientgen.PathToAliasForTest("go.digitalxero.dev/spacetimedb-client/client/cache"))
}

func TestDetectSpecialType(t *testing.T) {
	tests := []struct {
		name     string
		product  clientgen.ProductType
		expected string
	}{
		{
			name: "Identity",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__identity__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU256}},
				},
			},
			expected: "types.Identity",
		},
		{
			name: "ConnectionId",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__connection_id__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU128}},
				},
			},
			expected: "types.ConnectionId",
		},
		{
			name: "Timestamp",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__timestamp_micros_since_unix_epoch__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinI64}},
				},
			},
			expected: "types.Timestamp",
		},
		{
			name: "TimeDuration",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__time_duration_micros__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinI64}},
				},
			},
			expected: "types.TimeDuration",
		},
		{
			name: "UUID",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__uuid__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU128}},
				},
			},
			expected: "types.UUID",
		},
		{
			name: "Identity_wrong_builtin",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__identity__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU128}},
				},
			},
			expected: "",
		},
		{
			name: "multi_element_not_special",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "__identity__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU256}},
					{Name: "extra", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
				},
			},
			expected: "",
		},
		{
			name: "regular_product",
			product: clientgen.ProductType{
				Elements: []clientgen.ProductTypeElement{
					{Name: "id", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
				},
			},
			expected: "",
		},
		{
			name:     "empty_product",
			product:  clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clientgen.DetectSpecialTypeForTest(&tt.product)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSimpleEnum(t *testing.T) {
	t.Run("all_unit_variants", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "A", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}}}},
				{Name: "B", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}}}},
			},
		}
		assert.True(t, clientgen.IsSimpleEnumForTest(st))
	})

	t.Run("variant_with_fields", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "A", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}}}},
				{Name: "B", AlgebraicType: clientgen.AlgebraicType{
					Kind: clientgen.ATKProduct,
					Product: &clientgen.ProductType{
						Elements: []clientgen.ProductTypeElement{
							{Name: "value", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
						},
					},
				}},
			},
		}
		assert.False(t, clientgen.IsSimpleEnumForTest(st))
	})

	t.Run("non_product_variant", func(t *testing.T) {
		st := &clientgen.SumType{
			Variants: []clientgen.SumTypeVariant{
				{Name: "A", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
			},
		}
		assert.False(t, clientgen.IsSimpleEnumForTest(st))
	})
}
