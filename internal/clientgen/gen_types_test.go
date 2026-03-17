package clientgen_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/clientgen"
)

func TestGenerateTypes_EmptySchema(t *testing.T) {
	schema := &clientgen.ModuleSchema{}
	result, err := clientgen.GenerateTypesForTest(schema, "test_pkg")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGenerateTypes_SkipsSpecialTypes(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "__identity__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU256}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{
			{Name: "Identity", TypeRef: 0},
		},
	}
	result, err := clientgen.GenerateTypesForTest(schema, "test_pkg")
	require.NoError(t, err)
	assert.Nil(t, result) // Special types are skipped, so no output
}

func TestGenerateTypes_SkipsOptionAndScheduleAt(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKSum,
				Sum: &clientgen.SumType{
					Variants: []clientgen.SumTypeVariant{
						{Name: "some", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
						{Name: "none", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{}}},
					},
				},
			},
			{
				Kind: clientgen.ATKSum,
				Sum: &clientgen.SumType{
					Variants: []clientgen.SumTypeVariant{
						{Name: "Interval", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
						{Name: "Time", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{
			{Name: "OptionalName", TypeRef: 0},
			{Name: "ScheduleAt", TypeRef: 1},
		},
	}
	result, err := clientgen.GenerateTypesForTest(schema, "test_pkg")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGenerateTypes_SimpleEnum(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKSum,
				Sum: &clientgen.SumType{
					Variants: []clientgen.SumTypeVariant{
						{Name: "Active", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}}}},
						{Name: "Inactive", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}}}},
						{Name: "Suspended", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}}}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{
			{Name: "Status", TypeRef: 0},
		},
	}
	result, err := clientgen.GenerateTypesForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "type Status uint8")
	assert.Contains(t, code, "StatusActive Status = iota")
	assert.Contains(t, code, "StatusInactive")
	assert.Contains(t, code, "StatusSuspended")
	assert.Contains(t, code, "func (e Status) String() string")
}

func TestGenerateTypes_SumTypeInterface(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKSum,
				Sum: &clientgen.SumType{
					Variants: []clientgen.SumTypeVariant{
						{
							Name: "Text",
							AlgebraicType: clientgen.AlgebraicType{
								Kind: clientgen.ATKProduct,
								Product: &clientgen.ProductType{
									Elements: []clientgen.ProductTypeElement{
										{Name: "content", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
									},
								},
							},
						},
						{
							Name:          "Empty",
							AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}}},
						},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{
			{Name: "Message", TypeRef: 0},
		},
	}
	result, err := clientgen.GenerateTypesForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "type Message interface")
	assert.Contains(t, code, "messageVariant()")
	assert.Contains(t, code, "type MessageText struct")
	assert.Contains(t, code, "Content string")
	assert.Contains(t, code, "type MessageEmpty struct{}")
	assert.Contains(t, code, "func (MessageText) messageVariant()")
	assert.Contains(t, code, "func (MessageEmpty) messageVariant()")
}

func TestGenerateTypes_StructType(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "id", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
						{Name: "name", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{
			{Name: "Player", TypeRef: 0},
		},
	}
	result, err := clientgen.GenerateTypesForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "type Player struct")
	assert.Contains(t, code, "ID uint64")
	assert.Contains(t, code, "Name string")
}

func TestGenerateTypes_TableWithoutNamedType(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "value", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
		Tables: []clientgen.TableSchema{
			{
				Name:    "orphan_table",
				TypeRef: 0,
				ProductType: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "value", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
	}
	result, err := clientgen.GenerateTypesForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "type OrphanTable struct")
	assert.Contains(t, code, "Value string")
}

func TestGenerateTypes_TypesImportDetection(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			// 0: Identity special type
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "__identity__", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU256}},
					},
				},
			},
			// 1: struct with a ref to Identity
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "owner", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 0}},
						{Name: "name", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{
			{Name: "Item", TypeRef: 1},
		},
	}
	result, err := clientgen.GenerateTypesForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "types.Identity")
	assert.Contains(t, code, `"go.digitalxero.dev/spacetimedb-client/types"`)
}

func TestGenerateTypes_NilProductTypeTableSkipped(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{},
		Tables: []clientgen.TableSchema{
			{
				Name:        "bad_table",
				TypeRef:     99, // out of bounds
				ProductType: nil,
			},
		},
	}
	// Even though we have a table, the nil product type should cause it to be skipped
	result, err := clientgen.GenerateTypesForTest(schema, "test_pkg")
	require.NoError(t, err)
	// No types and the table is skipped due to nil ProductType
	assert.Nil(t, result)
}

func TestGenerateTypes_AllBuiltins(t *testing.T) {
	schema := loadTestSchema(t, "all_builtins")
	result, err := clientgen.GenerateTypesForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "type AllBuiltins struct")
	// Check all field names generated
	for _, field := range []string{"FBool", "FU8", "FU16", "FU32", "FU64", "FU128", "FU256",
		"FI8", "FI16", "FI32", "FI64", "FI128", "FI256", "FF32", "FF64", "FString", "FBytes"} {
		assert.True(t, strings.Contains(code, field), "expected field %s in generated code", field)
	}
}
