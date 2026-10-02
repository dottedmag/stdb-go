package clientgen

import (
	"fmt"
	"strings"
)

// generateTypes generates Go type definitions from the resolved schema.
func generateTypes(schema *ModuleSchema, pkgName string) ([]byte, error) {
	if len(schema.Types) == 0 && len(schema.Tables) == 0 && len(schema.Views) == 0 {
		return nil, nil
	}

	var w strings.Builder
	w.WriteString(clientFileHeader(pkgName))

	imports := map[string]string{}
	var typeCode strings.Builder

	// Track which typespace refs have been named
	namedRefs := map[int]string{}
	for _, t := range schema.Types {
		namedRefs[t.TypeRef] = t.Name
	}

	// Generate named types
	for _, t := range schema.Types {
		if t.TypeRef < 0 || t.TypeRef >= len(schema.Typespace) {
			continue
		}

		at := &schema.Typespace[t.TypeRef]
		goName := toGoName(t.Name)

		switch at.Kind {
		case ATKProduct:
			// Check for special types - skip them since the SDK provides them
			if specialType := detectSpecialType(at.Product); specialType != "" {
				continue
			}

			generateStructType(&typeCode, goName, at.Product, schema, imports)

		case ATKSum:
			if isOptionType(at.Sum) {
				// Skip Option types, they're handled inline
				continue
			}
			if isScheduleAtType(at.Sum) {
				// Skip ScheduleAt, provided by SDK
				continue
			}

			// Check if this is a simple enum (all unit variants)
			if isSimpleEnum(at.Sum) {
				generateEnumType(&typeCode, goName, at.Sum)
			} else {
				generateSumTypeInterface(&typeCode, goName, at.Sum, schema, imports)
			}
		}
	}

	// Generate table row types that don't have named type defs
	for _, table := range schema.Tables {
		if _, ok := namedRefs[table.TypeRef]; ok {
			// Already generated via named type
			continue
		}

		if table.ProductType == nil {
			continue
		}

		// Check for special types
		if specialType := detectSpecialType(table.ProductType); specialType != "" {
			continue
		}

		goName := toGoName(table.Name)
		generateStructType(&typeCode, goName, table.ProductType, schema, imports)
	}

	for _, view := range schema.Views {
		name, product, named, err := viewRow(view, schema)
		if err != nil {
			return nil, err
		}
		if !named {
			generateStructType(&typeCode, name, product, schema, imports)
		}
	}

	if typeCode.Len() == 0 {
		return nil, nil
	}

	w.WriteString(clientImportBlock(imports))
	w.WriteString(typeCode.String())

	return []byte(w.String()), nil
}

func generateStructType(w *strings.Builder, name string, pt *ProductType, schema *ModuleSchema, imports map[string]string) {
	fmt.Fprintf(w, "type %s struct {\n", name)
	for _, elem := range pt.Elements {
		goFieldName := toGoName(elem.Name)
		goType := goTypeForAlgebraic(&elem.AlgebraicType, schema.Typespace, schema.Types)

		if needsTypesImport(goType) {
			imports["types"] = "go.digitalxero.dev/spacetimedb-client/types"
		}

		fmt.Fprintf(w, "\t%s %s\n", goFieldName, goType)
	}
	w.WriteString("}\n\n")
}

func isSimpleEnum(st *SumType) bool {
	for _, v := range st.Variants {
		// A unit variant has a product type with no elements
		if v.AlgebraicType.Kind != ATKProduct || v.AlgebraicType.Product == nil || len(v.AlgebraicType.Product.Elements) != 0 {
			return false
		}
	}
	return true
}

func generateEnumType(w *strings.Builder, name string, st *SumType) {
	fmt.Fprintf(w, "type %s uint8\n\n", name)
	fmt.Fprintf(w, "const (\n")
	for i, v := range st.Variants {
		variantName := toGoName(v.Name)
		if i == 0 {
			fmt.Fprintf(w, "\t%s%s %s = iota\n", name, variantName, name)
		} else {
			fmt.Fprintf(w, "\t%s%s\n", name, variantName)
		}
	}
	fmt.Fprintf(w, ")\n\n")

	// String method
	fmt.Fprintf(w, "func (e %s) String() string {\n", name)
	fmt.Fprintf(w, "\tswitch e {\n")
	for _, v := range st.Variants {
		variantName := toGoName(v.Name)
		fmt.Fprintf(w, "\tcase %s%s:\n", name, variantName)
		fmt.Fprintf(w, "\t\treturn %q\n", v.Name)
	}
	fmt.Fprintf(w, "\tdefault:\n")
	fmt.Fprintf(w, "\t\treturn \"unknown\"\n")
	fmt.Fprintf(w, "\t}\n")
	fmt.Fprintf(w, "}\n\n")
}

func generateSumTypeInterface(w *strings.Builder, name string, st *SumType, schema *ModuleSchema, imports map[string]string) {
	// Interface
	lowerName := toLowerCamel(name)
	fmt.Fprintf(w, "type %s interface {\n", name)
	fmt.Fprintf(w, "\t%sVariant()\n", lowerName)
	fmt.Fprintf(w, "}\n\n")

	// Variant structs
	for _, v := range st.Variants {
		variantGoName := name + toGoName(v.Name)

		if v.AlgebraicType.Kind == ATKProduct && v.AlgebraicType.Product != nil && len(v.AlgebraicType.Product.Elements) > 0 {
			// Variant with fields
			fmt.Fprintf(w, "type %s struct {\n", variantGoName)
			for _, elem := range v.AlgebraicType.Product.Elements {
				goFieldName := toGoName(elem.Name)
				goType := goTypeForAlgebraic(&elem.AlgebraicType, schema.Typespace, schema.Types)
				if needsTypesImport(goType) {
					imports["types"] = "go.digitalxero.dev/spacetimedb-client/types"
				}
				fmt.Fprintf(w, "\t%s %s\n", goFieldName, goType)
			}
			w.WriteString("}\n\n")
		} else {
			// Unit variant
			fmt.Fprintf(w, "type %s struct{}\n\n", variantGoName)
		}

		// Implement interface
		fmt.Fprintf(w, "func (%s) %sVariant() {}\n\n", variantGoName, lowerName)
	}
}
