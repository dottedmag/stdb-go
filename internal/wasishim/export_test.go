package wasishim

// BuildShimWASMForTest exposes buildShimWASM for testing.
func BuildShimWASMForTest() []byte {
	return buildShimWASM()
}

// RenameExportForTest exposes renameExport for testing.
func RenameExportForTest(wasm []byte, oldName, newName string) ([]byte, error) {
	return renameExport(wasm, oldName, newName)
}

// RewriteWASIImportsForTest exposes rewriteWASIImports for testing.
func RewriteWASIImportsForTest(wasm []byte) ([]byte, error) {
	return rewriteWASIImports(wasm)
}
