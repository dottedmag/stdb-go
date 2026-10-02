package clientgen

import "fmt"

// viewRow resolves the product returned by a view, unwrapping arrays and options.
// The view's argument type and row type may have different names.
func viewRow(view ViewSchema, schema *ModuleSchema) (name string, product *ProductType, named bool, err error) {
	row := view.ReturnType
	ref := -1
	seen := make(map[int]bool)
	for row != nil {
		switch row.Kind {
		case ATKRef:
			ref = row.Ref
			if ref < 0 || ref >= len(schema.Typespace) || seen[ref] {
				return "", nil, false, fmt.Errorf("view %s: invalid row type reference %d", view.Name, ref)
			}
			seen[ref] = true
			row = &schema.Typespace[ref]
		case ATKArray:
			ref = -1
			row = row.ArrayTy
		case ATKSum:
			if row.Sum == nil || !isOptionType(row.Sum) {
				return "", nil, false, fmt.Errorf("view %s: return type must contain product rows", view.Name)
			}
			ref = -1
			row = &row.Sum.Variants[0].AlgebraicType
		case ATKProduct:
			if row.Product == nil {
				return "", nil, false, fmt.Errorf("view %s: missing row product", view.Name)
			}
			for _, typ := range schema.Types {
				if ref >= 0 && typ.TypeRef == ref {
					return toGoName(typ.Name), row.Product, true, nil
				}
			}
			return toGoName(view.Name), row.Product, false, nil
		default:
			return "", nil, false, fmt.Errorf("view %s: return type must contain product rows", view.Name)
		}
	}
	return "", nil, false, fmt.Errorf("view %s: missing return type", view.Name)
}
