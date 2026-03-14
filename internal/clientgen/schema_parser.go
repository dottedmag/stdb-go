package clientgen

import (
	"fmt"
)

// ModuleSchema is the resolved, ready-to-use module schema.
type ModuleSchema struct {
	Tables     []TableSchema
	Reducers   []ReducerSchema
	Procedures []ProcedureSchema
	Views      []ViewSchema
	Types      []TypeSchema
	Typespace  []AlgebraicType
}

// TableSchema is a resolved table definition.
type TableSchema struct {
	Name        string
	ProductType *ProductType
	PrimaryKey  []int
	Indexes     []IndexSchema
	Sequences   []SequenceSchema
	TableType   string // "User" or "System"
	Access      string // "Public" or "Private"
	IsEvent     bool
	TypeRef     int // index into Typespace
}

// IndexSchema is a resolved index definition.
type IndexSchema struct {
	Name         *string
	AccessorName *string
}

// SequenceSchema is a resolved auto-increment sequence.
type SequenceSchema struct {
	Name  *string
	ColID int
	Start *int64
}

// ReducerSchema is a resolved reducer definition.
type ReducerSchema struct {
	Name       string
	Params     []FieldSchema
	Visibility string // "Private" or "ClientCallable"
}

// ProcedureSchema is a resolved procedure definition.
type ProcedureSchema struct {
	Name       string
	Params     []FieldSchema
	ReturnType *AlgebraicType
	Visibility string
}

// ViewSchema is a resolved view definition.
type ViewSchema struct {
	Name        string
	Index       int
	IsPublic    bool
	IsAnonymous bool
	Params      []FieldSchema
	ReturnType  *AlgebraicType
}

// TypeSchema is a resolved named type definition.
type TypeSchema struct {
	Name           string
	Scope          []string
	TypeRef        int // index into Typespace
	CustomOrdering bool
}

// FieldSchema is a resolved field in a product type.
type FieldSchema struct {
	Name string
	Type *AlgebraicType
}

// resolveSchema converts a RawModuleDef into a resolved ModuleSchema.
func resolveSchema(raw *RawModuleDef) (*ModuleSchema, error) {
	if raw.V10 == nil {
		return nil, fmt.Errorf("only V10 module definitions are supported")
	}

	v10 := raw.V10
	schema := &ModuleSchema{}

	// First pass: extract typespace
	for _, section := range v10.Sections {
		if section.sectionType == "Typespace" {
			schema.Typespace = section.typespace
		}
	}

	// Second pass: resolve all other sections
	for _, section := range v10.Sections {
		switch section.sectionType {
		case "Types":
			for _, t := range section.types {
				schema.Types = append(schema.Types, TypeSchema{
					Name:           t.SourceName.SourceName,
					Scope:          t.SourceName.Scope,
					TypeRef:        t.Ty,
					CustomOrdering: t.CustomOrdering,
				})
			}

		case "Tables":
			for _, t := range section.tables {
				pk, err := t.ParsedPrimaryKey()
				if err != nil {
					return nil, fmt.Errorf("table %s: %w", t.SourceName, err)
				}

				// Resolve the product type from the typespace
				var productType *ProductType
				if t.ProductTypeRef >= 0 && t.ProductTypeRef < len(schema.Typespace) {
					at := &schema.Typespace[t.ProductTypeRef]
					if at.Kind == ATKProduct {
						productType = at.Product
					}
				}

				var indexes []IndexSchema
				for _, idx := range t.Indexes {
					indexes = append(indexes, IndexSchema{
						Name:         idx.SourceName,
						AccessorName: idx.AccessorName,
					})
				}

				var sequences []SequenceSchema
				for _, seq := range t.Sequences {
					sequences = append(sequences, SequenceSchema{
						Name:  seq.SourceName,
						ColID: seq.ColID,
						Start: seq.Start,
					})
				}

				schema.Tables = append(schema.Tables, TableSchema{
					Name:        t.SourceName,
					ProductType: productType,
					PrimaryKey:  pk,
					Indexes:     indexes,
					Sequences:   sequences,
					TableType:   t.TableType,
					Access:      t.TableAccess,
					IsEvent:     t.IsEvent,
					TypeRef:     t.ProductTypeRef,
				})
			}

		case "Reducers":
			for _, r := range section.reducers {
				schema.Reducers = append(schema.Reducers, ReducerSchema{
					Name:       r.SourceName,
					Params:     resolveParams(r.Params),
					Visibility: r.Visibility,
				})
			}

		case "Procedures":
			for _, p := range section.procedures {
				retType := p.ReturnType
				schema.Procedures = append(schema.Procedures, ProcedureSchema{
					Name:       p.SourceName,
					Params:     resolveParams(p.Params),
					ReturnType: &retType,
					Visibility: p.Visibility,
				})
			}

		case "Views":
			for _, v := range section.views {
				retType := v.ReturnType
				schema.Views = append(schema.Views, ViewSchema{
					Name:        v.SourceName,
					Index:       v.Index,
					IsPublic:    v.IsPublic,
					IsAnonymous: v.IsAnonymous,
					Params:      resolveParams(v.Params),
					ReturnType:  &retType,
				})
			}
		}
	}

	return schema, nil
}

func resolveParams(pt ProductType) []FieldSchema {
	fields := make([]FieldSchema, 0, len(pt.Elements))
	for _, elem := range pt.Elements {
		elemType := elem.AlgebraicType
		fields = append(fields, FieldSchema{
			Name: elem.Name,
			Type: &elemType,
		})
	}
	return fields
}
