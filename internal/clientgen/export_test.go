package clientgen

import "context"

// --- schema_parser.go exports ---

// ResolveSchemaForTest exposes resolveSchema for testing.
func ResolveSchemaForTest(raw *RawModuleDef) (*ModuleSchema, error) {
	return resolveSchema(raw)
}

// ResolveParamsForTest exposes resolveParams for testing.
func ResolveParamsForTest(pt ProductType) []FieldSchema {
	return resolveParams(pt)
}

// --- gen_common.go exports ---

// ToGoNameForTest exposes toGoName for testing.
func ToGoNameForTest(s string) string {
	return toGoName(s)
}

// ToLowerCamelForTest exposes toLowerCamel for testing.
func ToLowerCamelForTest(s string) string {
	return toLowerCamel(s)
}

// IsCommonAcronymForTest exposes isCommonAcronym for testing.
func IsCommonAcronymForTest(s string) bool {
	return isCommonAcronym(s)
}

// GoTypeForBuiltinForTest exposes goTypeForBuiltin for testing.
func GoTypeForBuiltinForTest(bt BuiltinType) string {
	return goTypeForBuiltin(bt)
}

// GoTypeForAlgebraicForTest exposes goTypeForAlgebraic for testing.
func GoTypeForAlgebraicForTest(at *AlgebraicType, typespace []AlgebraicType, types []TypeSchema) string {
	return goTypeForAlgebraic(at, typespace, types)
}

// GoTypeForRefForTest exposes goTypeForRef for testing.
func GoTypeForRefForTest(ref int, typespace []AlgebraicType, types []TypeSchema) string {
	return goTypeForRef(ref, typespace, types)
}

// GoTypeForSumForTest exposes goTypeForSum for testing.
func GoTypeForSumForTest(st *SumType, typespace []AlgebraicType, types []TypeSchema) string {
	return goTypeForSum(st, typespace, types)
}

// IsOptionTypeForTest exposes isOptionType for testing.
func IsOptionTypeForTest(st *SumType) bool {
	return isOptionType(st)
}

// IsScheduleAtTypeForTest exposes isScheduleAtType for testing.
func IsScheduleAtTypeForTest(st *SumType) bool {
	return isScheduleAtType(st)
}

// NeedsTypesImportForTest exposes needsTypesImport for testing.
func NeedsTypesImportForTest(typeStr string) bool {
	return needsTypesImport(typeStr)
}

// ClientFileHeaderForTest exposes clientFileHeader for testing.
func ClientFileHeaderForTest(pkgName string) string {
	return clientFileHeader(pkgName)
}

// ClientImportBlockForTest exposes clientImportBlock for testing.
func ClientImportBlockForTest(imports map[string]string) string {
	return clientImportBlock(imports)
}

// PathToAliasForTest exposes pathToAlias for testing.
func PathToAliasForTest(path string) string {
	return pathToAlias(path)
}

// DetectSpecialTypeForTest exposes detectSpecialType for testing.
func DetectSpecialTypeForTest(pt *ProductType) string {
	return detectSpecialType(pt)
}

// --- gen_types.go exports ---

// IsSimpleEnumForTest exposes isSimpleEnum for testing.
func IsSimpleEnumForTest(st *SumType) bool {
	return isSimpleEnum(st)
}

// GenerateTypesForTest exposes generateTypes for testing.
func GenerateTypesForTest(schema *ModuleSchema, pkgName string) ([]byte, error) {
	return generateTypes(schema, pkgName)
}

// --- gen_bsatn.go exports ---

// GenerateBsatnForTest exposes generateBsatn for testing.
func GenerateBsatnForTest(schema *ModuleSchema, pkgName string) ([]byte, error) {
	return generateBsatn(schema, pkgName)
}

// --- gen_tables.go exports ---

// GenerateTablesForTest exposes generateTables for testing.
func GenerateTablesForTest(schema *ModuleSchema, pkgName string) ([]byte, error) {
	return generateTables(schema, pkgName)
}

// --- gen_reducers.go exports ---

// GenerateReducersForTest exposes generateReducers for testing.
func GenerateReducersForTest(schema *ModuleSchema, pkgName string) ([]byte, error) {
	return generateReducers(schema, pkgName)
}

// --- gen_procedures.go exports ---

// GenerateProceduresForTest exposes generateProcedures for testing.
func GenerateProceduresForTest(schema *ModuleSchema, pkgName string) ([]byte, error) {
	return generateProcedures(schema, pkgName)
}

// --- gen_views.go exports ---

// GenerateViewsForTest exposes generateViews for testing.
func GenerateViewsForTest(schema *ModuleSchema, pkgName string) ([]byte, error) {
	return generateViews(schema, pkgName)
}

// --- gen_module.go exports ---

// GenerateModuleForTest exposes generateModule for testing.
func GenerateModuleForTest(schema *ModuleSchema, pkgName string) ([]byte, error) {
	return generateModule(schema, pkgName)
}

// --- clientgen.go exports ---

// FilteredSchemaForTest creates a clientGen with the given params and returns the filtered schema.
func FilteredSchemaForTest(schema *ModuleSchema, includePrivate bool) *ModuleSchema {
	g := &clientGen{
		schema:         schema,
		includePrivate: includePrivate,
	}
	return g.filteredSchema()
}

// --- extract.go exports ---

// BuildServerExtractorForTest builds a server extractor for testing.
func BuildServerExtractorForTest(serverURL, database, token, schemaVersion string) (SchemaExtractor, error) {
	b := NewSchemaExtractor().
		FromServer(serverURL, database, token)
	if schemaVersion != "" {
		b = b.WithSchemaVersion(schemaVersion)
	}
	return b.Build()
}

// ExtractFromServerForTest extracts a schema using the server extractor.
func ExtractFromServerForTest(ctx context.Context, serverURL, database, token string) (*ModuleSchema, error) {
	ext, err := BuildServerExtractorForTest(serverURL, database, token, "")
	if err != nil {
		return nil, err
	}
	return ext.Extract(ctx)
}
