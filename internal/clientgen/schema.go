package clientgen

import (
	"encoding/json"
	"fmt"
)

// RawModuleDef is the top-level envelope for a SpacetimeDB module definition.
// It wraps versioned definitions (V8, V9, V10).
type RawModuleDef struct {
	V10 *RawModuleDefV10 `json:"V10,omitempty"`
}

// RawModuleDefV10 is the V10 section-based module definition format.
type RawModuleDefV10 struct {
	Sections []RawModuleDefV10Section `json:"sections"`
}

// RawModuleDefV10Section is a discriminated union of section types.
type RawModuleDefV10Section struct {
	sectionType string
	typespace   []AlgebraicType
	types       []RawTypeDefV10
	tables      []RawTableDefV10
	reducers    []RawReducerDefV10
	procedures  []RawProcedureDefV10
	views       []RawViewDefV10
	schedules   []RawScheduleDefV10
	lifecycle   []RawLifeCycleReducerDefV10
	rls         []RawRowLevelSecurityDefV10
}

func (s *RawModuleDefV10Section) UnmarshalJSON(data []byte) error {
	// Sections are serialized as {"SectionName": <value>}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	for key, val := range raw {
		s.sectionType = key
		switch key {
		case "Typespace":
			return json.Unmarshal(val, &s.typespace)
		case "Types":
			return json.Unmarshal(val, &s.types)
		case "Tables":
			return json.Unmarshal(val, &s.tables)
		case "Reducers":
			return json.Unmarshal(val, &s.reducers)
		case "Procedures":
			return json.Unmarshal(val, &s.procedures)
		case "Views":
			return json.Unmarshal(val, &s.views)
		case "Schedules":
			return json.Unmarshal(val, &s.schedules)
		case "LifeCycleReducers":
			return json.Unmarshal(val, &s.lifecycle)
		case "RowLevelSecurity":
			return json.Unmarshal(val, &s.rls)
		case "CaseConversionPolicy", "ExplicitNames":
			// Ignored for client codegen
			return nil
		default:
			// Unknown section, skip
			return nil
		}
	}

	return nil
}

// --- Typespace types ---

// AlgebraicType is a discriminated union representing a type in the SATS type system.
type AlgebraicType struct {
	Kind     AlgebraicTypeKind
	Product  *ProductType   // Kind == ATKProduct
	Sum      *SumType       // Kind == ATKSum
	Builtin  BuiltinType    // Kind == ATKBuiltin (primitive name)
	Ref      int            // Kind == ATKRef (typespace index)
	ArrayTy  *AlgebraicType // Kind == ATKArray
	MapKey   *AlgebraicType // Kind == ATKMap
	MapValue *AlgebraicType // Kind == ATKMap
}

type AlgebraicTypeKind int

const (
	ATKProduct AlgebraicTypeKind = iota
	ATKSum
	ATKBuiltin
	ATKRef
	ATKArray
	ATKMap
)

func (at *AlgebraicType) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	for key, val := range raw {
		switch key {
		case "Product":
			at.Kind = ATKProduct
			at.Product = &ProductType{}
			return json.Unmarshal(val, at.Product)
		case "Sum":
			at.Kind = ATKSum
			at.Sum = &SumType{}
			return json.Unmarshal(val, at.Sum)
		case "Ref":
			at.Kind = ATKRef
			return json.Unmarshal(val, &at.Ref)
		case "Array":
			at.Kind = ATKArray
			at.ArrayTy = &AlgebraicType{}
			return json.Unmarshal(val, at.ArrayTy)
		case "Map":
			at.Kind = ATKMap
			var mapDef struct {
				Key   AlgebraicType `json:"key"`
				Value AlgebraicType `json:"value"`
			}
			if err := json.Unmarshal(val, &mapDef); err != nil {
				return err
			}
			at.MapKey = &mapDef.Key
			at.MapValue = &mapDef.Value
			return nil
		default:
			// Must be a builtin primitive type
			at.Kind = ATKBuiltin
			at.Builtin = BuiltinType(key)
			return nil
		}
	}

	return fmt.Errorf("empty algebraic type object")
}

// BuiltinType represents a primitive SATS type.
type BuiltinType string

const (
	BuiltinBool   BuiltinType = "Bool"
	BuiltinU8     BuiltinType = "U8"
	BuiltinU16    BuiltinType = "U16"
	BuiltinU32    BuiltinType = "U32"
	BuiltinU64    BuiltinType = "U64"
	BuiltinU128   BuiltinType = "U128"
	BuiltinU256   BuiltinType = "U256"
	BuiltinI8     BuiltinType = "I8"
	BuiltinI16    BuiltinType = "I16"
	BuiltinI32    BuiltinType = "I32"
	BuiltinI64    BuiltinType = "I64"
	BuiltinI128   BuiltinType = "I128"
	BuiltinI256   BuiltinType = "I256"
	BuiltinF32    BuiltinType = "F32"
	BuiltinF64    BuiltinType = "F64"
	BuiltinString BuiltinType = "String"
	BuiltinBytes  BuiltinType = "Bytes"
)

// ProductType represents a struct-like type with named fields.
type ProductType struct {
	Elements []ProductTypeElement `json:"elements"`
}

// ProductTypeElement is a single field in a product type.
type ProductTypeElement struct {
	Name          string        `json:"name"`
	AlgebraicType AlgebraicType `json:"algebraic_type"`
}

// SumType represents an enum/union type with named variants.
type SumType struct {
	Variants []SumTypeVariant `json:"variants"`
}

// SumTypeVariant is a single variant in a sum type.
type SumTypeVariant struct {
	Name          string        `json:"name"`
	AlgebraicType AlgebraicType `json:"algebraic_type"`
}

// --- Table definitions ---

// RawTableDefV10 defines a table in the module schema.
type RawTableDefV10 struct {
	SourceName     string              `json:"source_name"`
	ProductTypeRef int                 `json:"product_type_ref"`
	PrimaryKey     json.RawMessage     `json:"primary_key"`
	Indexes        []RawIndexDefV10    `json:"indexes"`
	Constraints    []json.RawMessage   `json:"constraints"`
	Sequences      []RawSequenceDefV10 `json:"sequences"`
	TableType      string              `json:"table_type"`
	TableAccess    string              `json:"table_access"`
	DefaultValues  []json.RawMessage   `json:"default_values"`
	IsEvent        bool                `json:"is_event"`
}

// ParsedPrimaryKey extracts the primary key column indices from the raw JSON.
func (t *RawTableDefV10) ParsedPrimaryKey() ([]int, error) {
	if t.PrimaryKey == nil {
		return nil, nil
	}

	// Primary key can be a single int or an array
	var single int
	if err := json.Unmarshal(t.PrimaryKey, &single); err == nil {
		return []int{single}, nil
	}

	var multi []int
	if err := json.Unmarshal(t.PrimaryKey, &multi); err == nil {
		return multi, nil
	}

	return nil, fmt.Errorf("invalid primary_key format: %s", string(t.PrimaryKey))
}

// RawIndexDefV10 defines an index on a table.
type RawIndexDefV10 struct {
	SourceName   *string         `json:"source_name"`
	AccessorName *string         `json:"accessor_name"`
	Algorithm    json.RawMessage `json:"algorithm"`
}

// RawSequenceDefV10 defines an auto-increment sequence.
type RawSequenceDefV10 struct {
	SourceName *string `json:"source_name"`
	ColID      int     `json:"col_id"`
	Start      *int64  `json:"start"`
}

// --- Reducer definitions ---

// RawReducerDefV10 defines a reducer (server-side function).
type RawReducerDefV10 struct {
	SourceName    string        `json:"source_name"`
	Params        ProductType   `json:"params"`
	Visibility    string        `json:"visibility"`
	OkReturnType  AlgebraicType `json:"ok_return_type"`
	ErrReturnType AlgebraicType `json:"err_return_type"`
}

// --- Procedure definitions ---

// RawProcedureDefV10 defines a procedure.
type RawProcedureDefV10 struct {
	SourceName string        `json:"source_name"`
	Params     ProductType   `json:"params"`
	ReturnType AlgebraicType `json:"return_type"`
	Visibility string        `json:"visibility"`
}

// --- View definitions ---

// RawViewDefV10 defines a view (server-side query).
type RawViewDefV10 struct {
	SourceName  string        `json:"source_name"`
	Index       int           `json:"index"`
	IsPublic    bool          `json:"is_public"`
	IsAnonymous bool          `json:"is_anonymous"`
	Params      ProductType   `json:"params"`
	ReturnType  AlgebraicType `json:"return_type"`
}

// --- Type definitions ---

// RawTypeDefV10 defines a named type in the module schema.
type RawTypeDefV10 struct {
	SourceName     RawScopedTypeNameV10 `json:"source_name"`
	Ty             int                  `json:"ty"`
	CustomOrdering bool                 `json:"custom_ordering"`
}

// RawScopedTypeNameV10 contains the scoped name of a type.
type RawScopedTypeNameV10 struct {
	Scope      []string `json:"scope"`
	SourceName string   `json:"source_name"`
}

// --- Schedule definitions ---

// RawScheduleDefV10 defines a scheduled reducer.
type RawScheduleDefV10 struct {
	SourceName    *string `json:"source_name"`
	TableName     string  `json:"table_name"`
	ScheduleAtCol int     `json:"schedule_at_col"`
	FunctionName  string  `json:"function_name"`
}

// --- Lifecycle reducer definitions ---

// RawLifeCycleReducerDefV10 defines a lifecycle reducer.
type RawLifeCycleReducerDefV10 struct {
	LifecycleSpec string `json:"lifecycle_spec"`
	FunctionName  string `json:"function_name"`
}

// --- Row-level security definitions ---

// RawRowLevelSecurityDefV10 defines row-level security.
type RawRowLevelSecurityDefV10 struct {
	TableName string `json:"table_name"`
	SQL       string `json:"sql"`
}

// --- Special type tag constants ---

const (
	IdentityTag           = "__identity__"
	ConnectionIDTag       = "__connection_id__"
	TimestampTag          = "__timestamp_micros_since_unix_epoch__"
	TimeDurationTag       = "__time_duration_micros__"
	UUIDTag               = "__uuid__"
	ScheduleAtTimeTag     = "Time"
	ScheduleAtIntervalTag = "Interval"
)
