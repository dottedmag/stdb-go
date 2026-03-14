package clientgen

// ResolveSchemaForTest exposes resolveSchema for testing.
func ResolveSchemaForTest(raw *RawModuleDef) (*ModuleSchema, error) {
	return resolveSchema(raw)
}

// ToGoNameForTest exposes toGoName for testing.
func ToGoNameForTest(s string) string {
	return toGoName(s)
}

// DetectSpecialTypeForTest exposes detectSpecialType for testing.
func DetectSpecialTypeForTest(pt *ProductType) string {
	return detectSpecialType(pt)
}
