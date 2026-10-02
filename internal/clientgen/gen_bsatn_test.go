package clientgen_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/clientgen"
)

func TestGenerateBsatn_EmptySchema(t *testing.T) {
	schema := &clientgen.ModuleSchema{}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGenerateBsatn_EmptyProductCompiles(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{}}},
		Types:     []clientgen.TypeSchema{{Name: "Empty", TypeRef: 0}},
		Reducers: []clientgen.ReducerSchema{{Name: "use_empty", Params: []clientgen.FieldSchema{
			{Name: "value", Type: &clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 0}},
		}}},
	}
	gen, err := clientgen.NewClientGen().WithSchema(schema).WithPackageName("bindings").Build()
	require.NoError(t, err)
	files, err := gen.Generate()
	require.NoError(t, err)

	// Empty codecs need only the Reader/Writer type names. Type-checking catches
	// unused locals, which gofmt and the existing golden tests cannot detect.
	fset := token.NewFileSet()
	stub, err := parser.ParseFile(fset, "bsatn.go", `package bsatn; type Reader interface{}; type Writer interface{}`, 0)
	require.NoError(t, err)
	bsatn, err := (&types.Config{}).Check("go.digitalxero.dev/spacetimedb-client/bsatn", fset, []*ast.File{stub}, nil)
	require.NoError(t, err)
	var codecs []*ast.File
	for _, file := range files {
		if file.Name != "types_generated.go" && file.Name != "bsatn_generated.go" {
			continue
		}
		parsed, err := parser.ParseFile(fset, file.Name, file.Content, 0)
		require.NoError(t, err)
		codecs = append(codecs, parsed)
	}
	require.Len(t, codecs, 2)
	_, err = (&types.Config{Importer: codecTestImporter{bsatn}}).Check("bindings", fset, codecs, nil)
	require.NoError(t, err)
}

type codecTestImporter struct {
	bsatn *types.Package
}

func (i codecTestImporter) Import(path string) (*types.Package, error) {
	if path == i.bsatn.Path() {
		return i.bsatn, nil
	}
	return nil, fmt.Errorf("unexpected import %q", path)
}

func TestGenerateBsatn_StructEncodeDecode(t *testing.T) {
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
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "func (v *Player) WriteBsatn(w bsatn.Writer)")
	assert.Contains(t, code, "w.PutU64(v.ID)")
	assert.Contains(t, code, "w.PutString(v.Name)")
	assert.Contains(t, code, "func ReadPlayer(r bsatn.Reader) (*Player, error)")
	assert.Contains(t, code, "v.ID, err = r.GetU64()")
	assert.Contains(t, code, "v.Name, err = r.GetString()")
}

func TestGenerateBsatn_AllBuiltinEncoders(t *testing.T) {
	elems := []clientgen.ProductTypeElement{
		{Name: "f_bool", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinBool}},
		{Name: "f_u8", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU8}},
		{Name: "f_u16", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU16}},
		{Name: "f_u32", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU32}},
		{Name: "f_u64", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU64}},
		{Name: "f_u128", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU128}},
		{Name: "f_u256", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU256}},
		{Name: "f_i8", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinI8}},
		{Name: "f_i16", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinI16}},
		{Name: "f_i32", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinI32}},
		{Name: "f_i64", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinI64}},
		{Name: "f_i128", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinI128}},
		{Name: "f_i256", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinI256}},
		{Name: "f_f32", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinF32}},
		{Name: "f_f64", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinF64}},
		{Name: "f_string", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
		{Name: "f_bytes", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinBytes}},
	}

	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: elems}},
		},
		Types: []clientgen.TypeSchema{{Name: "AllTypes", TypeRef: 0}},
	}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)

	// Verify encoders
	assert.Contains(t, code, "w.PutBool(v.FBool)")
	assert.Contains(t, code, "w.PutU8(v.FU8)")
	assert.Contains(t, code, "w.PutU16(v.FU16)")
	assert.Contains(t, code, "w.PutU32(v.FU32)")
	assert.Contains(t, code, "w.PutU64(v.FU64)")
	assert.Contains(t, code, "v.FU128.WriteBsatn(w)")
	assert.Contains(t, code, "v.FU256.WriteBsatn(w)")
	assert.Contains(t, code, "w.PutI8(v.FI8)")
	assert.Contains(t, code, "w.PutI16(v.FI16)")
	assert.Contains(t, code, "w.PutI32(v.FI32)")
	assert.Contains(t, code, "w.PutI64(v.FI64)")
	assert.Contains(t, code, "v.FI128.WriteBsatn(w)")
	assert.Contains(t, code, "v.FI256.WriteBsatn(w)")
	assert.Contains(t, code, "w.PutF32(v.FF32)")
	assert.Contains(t, code, "w.PutF64(v.FF64)")
	assert.Contains(t, code, "w.PutString(v.FString)")
	assert.Contains(t, code, "bsatn.WriteByteArray(w, v.FBytes)")

	// Verify decoders
	assert.Contains(t, code, "v.FBool, err = r.GetBool()")
	assert.Contains(t, code, "v.FU8, err = r.GetU8()")
	assert.Contains(t, code, "v.FU64, err = r.GetU64()")
	assert.Contains(t, code, "v.FU128, err = types.ReadU128(r)")
	assert.Contains(t, code, "v.FU256, err = types.ReadU256(r)")
	assert.Contains(t, code, "v.FI128, err = types.ReadI128(r)")
	assert.Contains(t, code, "v.FI256, err = types.ReadI256(r)")
	assert.Contains(t, code, "v.FF32, err = r.GetF32()")
	assert.Contains(t, code, "v.FString, err = r.GetString()")
	assert.Contains(t, code, "v.FBytes, err = bsatn.ReadByteArray(r)")
}

func TestGenerateBsatn_EnumEncodeDecode(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKSum,
				Sum: &clientgen.SumType{
					Variants: []clientgen.SumTypeVariant{
						{Name: "Active", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}}}},
						{Name: "Inactive", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}}}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{{Name: "Status", TypeRef: 0}},
	}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "func (v Status) WriteBsatn(w bsatn.Writer)")
	assert.Contains(t, code, "w.PutU8(uint8(v))")
	assert.Contains(t, code, "func ReadStatus(r bsatn.Reader) (Status, error)")
	assert.Contains(t, code, "tag, err := r.GetU8()")
	assert.Contains(t, code, "return Status(tag), nil")
}

func TestGenerateBsatn_SumTypeEncodeDecode(t *testing.T) {
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
		Types: []clientgen.TypeSchema{{Name: "Message", TypeRef: 0}},
	}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "func WriteMessage(w bsatn.Writer, v Message)")
	assert.Contains(t, code, "case MessageText:")
	assert.Contains(t, code, "w.PutSumTag(0)")
	assert.Contains(t, code, "val.WriteBsatn(w)")
	assert.Contains(t, code, "case MessageEmpty:")
	assert.Contains(t, code, "bsatn.WriteSumUnit(w, 1)")
	assert.Contains(t, code, "func ReadMessage(r bsatn.Reader) (Message, error)")
	assert.Contains(t, code, "func (v *MessageText) WriteBsatn(w bsatn.Writer)")
	assert.Contains(t, code, "func ReadMessageText(r bsatn.Reader) (*MessageText, error)")
}

func TestGenerateBsatn_SkipsSpecialTypes(t *testing.T) {
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
		Types: []clientgen.TypeSchema{{Name: "Identity", TypeRef: 0}},
	}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGenerateBsatn_SkipsOptionAndScheduleAt(t *testing.T) {
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
		},
		Types: []clientgen.TypeSchema{{Name: "OptName", TypeRef: 0}},
	}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGenerateBsatn_OptionFieldEncodeDecode(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "name", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
						{
							Name: "description",
							AlgebraicType: clientgen.AlgebraicType{
								Kind: clientgen.ATKSum,
								Sum: &clientgen.SumType{
									Variants: []clientgen.SumTypeVariant{
										{Name: "some", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString}},
										{Name: "none", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{}}},
									},
								},
							},
						},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{{Name: "Item", TypeRef: 0}},
	}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	// Option encoder
	assert.Contains(t, code, "if v.Description != nil")
	assert.Contains(t, code, "w.PutSumTag(0) // Some")
	assert.Contains(t, code, "w.PutSumTag(1) // None")
	// Option decoder
	assert.Contains(t, code, "tag, err = r.GetSumTag()")
	assert.Contains(t, code, "if tag == 0 { // Some")
}

func TestGenerateBsatn_ArrayU8(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{
							Name: "data",
							AlgebraicType: clientgen.AlgebraicType{
								Kind:    clientgen.ATKArray,
								ArrayTy: &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinU8},
							},
						},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{{Name: "BlobHolder", TypeRef: 0}},
	}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	// U8 arrays use WriteByteArray/ReadByteArray
	assert.Contains(t, code, "bsatn.WriteByteArray(w, v.Data)")
	assert.Contains(t, code, "v.Data, err = bsatn.ReadByteArray(r)")
}

func TestGenerateBsatn_ArrayOther(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{
							Name: "tags",
							AlgebraicType: clientgen.AlgebraicType{
								Kind:    clientgen.ATKArray,
								ArrayTy: &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString},
							},
						},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{{Name: "TagHolder", TypeRef: 0}},
	}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	// Non-U8 arrays use PutArrayLen
	assert.Contains(t, code, "w.PutArrayLen(uint32(len(v.Tags)))")
	assert.Contains(t, code, "for _, elem := range v.Tags")
	assert.Contains(t, code, "w.PutString(elem)")
	// Decoder
	assert.Contains(t, code, "arrLen, err = r.GetArrayLen()")
	assert.Contains(t, code, "v.Tags = make([]string, arrLen)")
}

func TestGenerateBsatn_Map(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{
							Name: "metadata",
							AlgebraicType: clientgen.AlgebraicType{
								Kind:     clientgen.ATKMap,
								MapKey:   &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString},
								MapValue: &clientgen.AlgebraicType{Kind: clientgen.ATKBuiltin, Builtin: clientgen.BuiltinString},
							},
						},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{{Name: "Meta", TypeRef: 0}},
	}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	// Encoder
	assert.Contains(t, code, "w.PutMapLen(uint32(len(v.Metadata)))")
	assert.Contains(t, code, "for k, v := range v.Metadata")
	assert.Contains(t, code, "w.PutString(k)")
	assert.Contains(t, code, "w.PutString(v)")
	// Decoder
	assert.Contains(t, code, "mapLen, err = r.GetMapLen()")
	assert.Contains(t, code, "v.Metadata = make(map[string]string, mapLen)")
}

func TestGenerateBsatn_TableWithoutNamedType(t *testing.T) {
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
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	assert.Contains(t, code, "func (v *OrphanTable) WriteBsatn(w bsatn.Writer)")
	assert.Contains(t, code, "func ReadOrphanTable(r bsatn.Reader) (*OrphanTable, error)")
}

func TestGenerateBsatn_RefEncoder(t *testing.T) {
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
			// 1: struct referencing identity
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
		Types: []clientgen.TypeSchema{{Name: "OwnedItem", TypeRef: 1}},
	}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	// Ref to special type should use WriteBsatn pattern
	assert.Contains(t, code, "v.Owner.WriteBsatn(w)")
	// Decoder should use special type reader
	assert.Contains(t, code, "v.Owner, err = types.ReadIdentity(r)")
}

func TestGenerateBsatn_RefEnumEncoder(t *testing.T) {
	schema := &clientgen.ModuleSchema{
		Typespace: []clientgen.AlgebraicType{
			// 0: simple enum
			{
				Kind: clientgen.ATKSum,
				Sum: &clientgen.SumType{
					Variants: []clientgen.SumTypeVariant{
						{Name: "A", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}}}},
						{Name: "B", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKProduct, Product: &clientgen.ProductType{Elements: []clientgen.ProductTypeElement{}}}},
					},
				},
			},
			// 1: struct referencing enum
			{
				Kind: clientgen.ATKProduct,
				Product: &clientgen.ProductType{
					Elements: []clientgen.ProductTypeElement{
						{Name: "status", AlgebraicType: clientgen.AlgebraicType{Kind: clientgen.ATKRef, Ref: 0}},
					},
				},
			},
		},
		Types: []clientgen.TypeSchema{
			{Name: "MyEnum", TypeRef: 0},
			{Name: "MyStruct", TypeRef: 1},
		},
	}
	result, err := clientgen.GenerateBsatnForTest(schema, "test_pkg")
	require.NoError(t, err)
	require.NotNil(t, result)

	code := string(result)
	// Enum ref should use PutU8/GetU8
	assert.Contains(t, code, "w.PutU8(uint8(v.Status))")
	assert.Contains(t, code, "tag, err = r.GetU8()")
}
