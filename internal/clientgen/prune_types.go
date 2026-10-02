package clientgen

// pruneUnusedTypes keeps named types reachable from the client API. Typespace
// indices stay unchanged so references remain valid, including recursive types.
func pruneUnusedTypes(schema *ModuleSchema) *ModuleSchema {
	reachable := make(map[int]bool)
	var visit func(*AlgebraicType)
	visit = func(at *AlgebraicType) {
		if at == nil {
			return
		}
		switch at.Kind {
		case ATKRef:
			if at.Ref < 0 || at.Ref >= len(schema.Typespace) || reachable[at.Ref] {
				return
			}
			reachable[at.Ref] = true
			visit(&schema.Typespace[at.Ref])
		case ATKProduct:
			if at.Product != nil {
				for i := range at.Product.Elements {
					visit(&at.Product.Elements[i].AlgebraicType)
				}
			}
		case ATKSum:
			if at.Sum != nil {
				for i := range at.Sum.Variants {
					visit(&at.Sum.Variants[i].AlgebraicType)
				}
			}
		case ATKArray:
			visit(at.ArrayTy)
		case ATKMap:
			visit(at.MapKey)
			visit(at.MapValue)
		}
	}

	visitParams := func(params []FieldSchema) {
		for _, param := range params {
			visit(param.Type)
		}
	}
	for _, table := range schema.Tables {
		visit(&AlgebraicType{Kind: ATKRef, Ref: table.TypeRef})
		visit(&AlgebraicType{Kind: ATKProduct, Product: table.ProductType})
	}
	for _, reducer := range schema.Reducers {
		visitParams(reducer.Params)
	}
	for _, procedure := range schema.Procedures {
		visitParams(procedure.Params)
		visit(procedure.ReturnType)
	}
	for _, view := range schema.Views {
		visitParams(view.Params)
		visit(view.ReturnType)
	}

	filtered := *schema
	filtered.Types = nil
	for _, typ := range schema.Types {
		if reachable[typ.TypeRef] {
			filtered.Types = append(filtered.Types, typ)
		}
	}
	return &filtered
}
