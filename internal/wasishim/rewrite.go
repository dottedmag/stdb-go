package wasishim

import (
	"fmt"
	"os"
)

// wasiModule is the WASI Preview 1 module name used as an import namespace.
const wasiModule = "wasi_snapshot_preview1"

// wasiImportInfo records a WASI function import that will be replaced with a local stub.
type wasiImportInfo struct {
	name    string // function name (e.g. "fd_write")
	typeIdx uint32 // type index in the original type section
	funcIdx uint32 // original function index (position among all imports)
}

// importEntry represents a single import in the WASM import section.
type importEntry struct {
	module  string
	name    string
	kind    byte   // 0=func, 1=table, 2=memory, 3=global
	rawDesc []byte // the descriptor bytes after the kind byte
	typeIdx uint32 // only meaningful for kind==0 (function imports)
}

// RewriteWASI reads a WASM binary at wasmPath, replaces all wasi_snapshot_preview1
// imports with local stub functions, renames _start to _initialize if present,
// and writes the result back.
func RewriteWASI(wasmPath string) error {
	wasm, err := os.ReadFile(wasmPath)
	if err != nil {
		return fmt.Errorf("read wasm: %w", err)
	}

	patched, err := rewriteWASIImports(wasm)
	if err != nil {
		return fmt.Errorf("rewrite wasi imports: %w", err)
	}

	// Wrap __preinit__* exports to call _initialize first, ensuring the Go
	// runtime is set up before any wasmexport trampoline executes. This is
	// needed for SpacetimeDB hosts that don't call _initialize themselves.
	patched, err = wrapPreinitWithInit(patched)
	if err != nil {
		return fmt.Errorf("wrap preinit functions: %w", err)
	}

	if err := os.WriteFile(wasmPath, patched, 0o644); err != nil {
		return fmt.Errorf("write patched wasm: %w", err)
	}

	return nil
}

// rewriteWASIImports is the core rewriter. It removes wasi_snapshot_preview1
// function imports, appends local stub implementations, and remaps all function
// indices throughout the module.
func rewriteWASIImports(wasm []byte) ([]byte, error) {
	if len(wasm) < 8 || string(wasm[:4]) != "\x00asm" {
		return nil, fmt.Errorf("invalid WASM module")
	}

	// Phase A: Parse all sections to extract the info we need.
	sections, err := parseSections(wasm)
	if err != nil {
		return nil, err
	}

	// Parse imports to find WASI function imports.
	importSec := findSection(sections, sectionImport)
	if importSec == nil {
		return wasm, nil // no imports, nothing to do
	}

	imports, err := parseImports(importSec.data)
	if err != nil {
		return nil, fmt.Errorf("parse imports: %w", err)
	}

	// Identify WASI function imports and count other import types.
	var wasiImports []wasiImportInfo
	var funcImportIdx uint32
	var keptImports []importEntry
	totalImportFuncs := uint32(0)

	for _, imp := range imports {
		if imp.kind == 0x00 { // function import
			if imp.module == wasiModule {
				wasiImports = append(wasiImports, wasiImportInfo{
					name:    imp.name,
					typeIdx: imp.typeIdx,
					funcIdx: funcImportIdx,
				})
			} else {
				keptImports = append(keptImports, imp)
			}
			funcImportIdx++
			totalImportFuncs++
		} else {
			keptImports = append(keptImports, imp)
		}
	}

	if len(wasiImports) == 0 {
		// No WASI imports found; just do the _start rename.
		return renameStartIfPresent(wasm)
	}

	// Parse existing types.
	typeSec := findSection(sections, sectionType)
	var existingTypes []funcType
	if typeSec != nil {
		existingTypes, err = parseTypes(typeSec.data)
		if err != nil {
			return nil, fmt.Errorf("parse types: %w", err)
		}
	}

	// Count non-import functions from the function section.
	funcSec := findSection(sections, sectionFunc)
	var localFuncTypeIndices []uint32
	if funcSec != nil {
		localFuncTypeIndices, err = parseFuncSection(funcSec.data)
		if err != nil {
			return nil, fmt.Errorf("parse func section: %w", err)
		}
	}

	// Phase B: Build index remapping.
	//
	// Original layout: [import_func_0 .. import_func_N-1] [local_func_0 .. local_func_M-1]
	// After removing K WASI imports:
	//   [kept_import_funcs] [local_funcs] [stub_funcs]
	//
	// Non-WASI import funcs keep their relative order but shift down.
	// Local funcs shift down by K (the number of removed WASI imports).
	// Stub funcs go at the end.

	removedCount := uint32(len(wasiImports))
	keptImportFuncCount := totalImportFuncs - removedCount
	totalLocalFuncs := uint32(len(localFuncTypeIndices))
	newTotalFuncs := keptImportFuncCount + totalLocalFuncs + removedCount // stubs appended

	// Build the function index remap table.
	// oldIdx -> newIdx for every function in the original module.
	funcRemap := make(map[uint32]uint32)

	// First pass: assign new indices to kept import functions.
	newIdx := uint32(0)
	oldImportFuncIdx := uint32(0)
	wasiSet := make(map[uint32]int) // oldFuncIdx -> index in wasiImports
	for i, wi := range wasiImports {
		wasiSet[wi.funcIdx] = i
	}

	for _, imp := range imports {
		if imp.kind != 0x00 {
			continue
		}
		if _, isWasi := wasiSet[oldImportFuncIdx]; !isWasi {
			funcRemap[oldImportFuncIdx] = newIdx
			newIdx++
		}
		oldImportFuncIdx++
	}

	// Local functions: they originally start at totalImportFuncs, now start at keptImportFuncCount.
	for i := uint32(0); i < totalLocalFuncs; i++ {
		oldIdx := totalImportFuncs + i
		funcRemap[oldIdx] = keptImportFuncCount + i
	}

	// Stub functions: placed after all local functions.
	stubStartIdx := keptImportFuncCount + totalLocalFuncs
	for i, wi := range wasiImports {
		funcRemap[wi.funcIdx] = stubStartIdx + uint32(i)
	}

	_ = newTotalFuncs // used implicitly

	// Phase C: Build stub function types, type indices, and code bodies.
	// We need to match each WASI import's type signature to generate the right stub.
	stubTypes, stubTypeIndices, stubBodies, stubGlobals, err := buildStubs(wasiImports, existingTypes)
	if err != nil {
		return nil, fmt.Errorf("build stubs: %w", err)
	}

	// Merge new types into existing types, deduplicating.
	typeRemap := make(map[int]uint32) // stubTypes index -> final type index
	for i, st := range stubTypes {
		found := false
		for j, et := range existingTypes {
			if typesEqual(st, et) {
				typeRemap[i] = uint32(j)
				found = true
				break
			}
		}
		if !found {
			typeRemap[i] = uint32(len(existingTypes))
			existingTypes = append(existingTypes, st)
		}
	}

	// Resolve stub type indices to final type indices.
	resolvedStubTypeIndices := make([]uint32, len(stubTypeIndices))
	for i, sti := range stubTypeIndices {
		resolvedStubTypeIndices[i] = typeRemap[int(sti)]
	}

	// Count existing globals to assign correct indices for stub globals.
	globalSec := findSection(sections, sectionGlobal)
	existingGlobalCount := uint32(0)
	if globalSec != nil {
		existingGlobalCount, _ = readULEB128(globalSec.data)
	}

	// Build global index remap for stub code bodies.
	// Stub globals are appended after existing globals.
	globalIdxBase := existingGlobalCount

	// Phase D: Rebuild the module section by section.
	var result []byte
	result = append(result, wasm[:8]...) // magic + version

	for _, sec := range sections {
		switch sec.id {
		case sectionType:
			result = appendRebuiltTypeSection(result, existingTypes)

		case sectionImport:
			result = appendRebuiltImportSection(result, keptImports)

		case sectionFunc:
			result = appendRebuiltFuncSection(result, localFuncTypeIndices, resolvedStubTypeIndices)

		case sectionGlobal:
			result = appendRebuiltGlobalSection(result, sec.data, stubGlobals)

		case sectionExport:
			newExpSec, err := rebuildExportSectionWithRemap(sec.data, funcRemap)
			if err != nil {
				return nil, fmt.Errorf("rebuild exports: %w", err)
			}
			result = append(result, sectionExport)
			result = appendULEB128(result, uint32(len(newExpSec)))
			result = append(result, newExpSec...)

		case sectionStart:
			result = appendRemappedStartSection(result, sec.data, funcRemap)

		case sectionElement:
			newElemData, err := remapElementSection(sec.data, funcRemap)
			if err != nil {
				return nil, fmt.Errorf("remap elements: %w", err)
			}
			result = append(result, sectionElement)
			result = appendULEB128(result, uint32(len(newElemData)))
			result = append(result, newElemData...)

		case sectionCode:
			newCodeData, err := rebuildCodeSection(sec.data, funcRemap, globalIdxBase, stubBodies)
			if err != nil {
				return nil, fmt.Errorf("rebuild code: %w", err)
			}
			result = append(result, sectionCode)
			result = appendULEB128(result, uint32(len(newCodeData)))
			result = append(result, newCodeData...)

		default:
			// Table, Memory, Data, DataCount, Custom — copy as-is.
			result = append(result, sec.id)
			result = appendULEB128(result, uint32(len(sec.data)))
			result = append(result, sec.data...)
		}
	}

	// If there was no global section originally but we have stub globals, add one.
	if globalSec == nil && len(stubGlobals) > 0 {
		var gSec []byte
		gSec = appendULEB128(gSec, uint32(len(stubGlobals)))
		for _, g := range stubGlobals {
			gSec = append(gSec, g...)
		}
		// Insert before export section — but since we already built result linearly,
		// we need to handle this differently. Actually the loop above handles all sections
		// in order, so if there's no global section, we need to inject it.
		// For simplicity, rebuild result with the global section inserted at the right spot.
		result = insertGlobalSection(result, gSec)
	}

	return result, nil
}

// rawSection holds a parsed WASM section.
type rawSection struct {
	id   byte
	data []byte
}

// parseSections extracts all sections from a WASM binary.
func parseSections(wasm []byte) ([]rawSection, error) {
	pos := 8 // skip magic + version
	var sections []rawSection

	for pos < len(wasm) {
		id := wasm[pos]
		pos++

		size, n := readULEB128(wasm[pos:])
		pos += n

		if pos+int(size) > len(wasm) {
			return nil, fmt.Errorf("section %d: size %d exceeds module (at offset %d)", id, size, pos)
		}

		sections = append(sections, rawSection{
			id:   id,
			data: wasm[pos : pos+int(size)],
		})
		pos += int(size)
	}

	return sections, nil
}

func findSection(sections []rawSection, id byte) *rawSection {
	for i := range sections {
		if sections[i].id == id {
			return &sections[i]
		}
	}
	return nil
}

// parseImports parses the import section data into importEntry structs.
func parseImports(data []byte) ([]importEntry, error) {
	pos := 0
	count, n := readULEB128(data[pos:])
	pos += n

	entries := make([]importEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		modLen, n := readULEB128(data[pos:])
		pos += n
		module := string(data[pos : pos+int(modLen)])
		pos += int(modLen)

		nameLen, n := readULEB128(data[pos:])
		pos += n
		name := string(data[pos : pos+int(nameLen)])
		pos += int(nameLen)

		kind := data[pos]
		pos++

		entry := importEntry{module: module, name: name, kind: kind}

		switch kind {
		case 0x00: // function: typeidx
			typeIdx, n := readULEB128(data[pos:])
			pos += n
			entry.typeIdx = typeIdx
			entry.rawDesc = nil // not needed, we have typeIdx
		case 0x01: // table: elemtype limits
			start := pos
			pos++ // elemtype
			pos = skipLimits(data, pos)
			entry.rawDesc = data[start:pos]
		case 0x02: // memory: limits
			start := pos
			pos = skipLimits(data, pos)
			entry.rawDesc = data[start:pos]
		case 0x03: // global: valtype mut
			start := pos
			pos += 2 // valtype + mut
			entry.rawDesc = data[start:pos]
		default:
			return nil, fmt.Errorf("unknown import kind %d", kind)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// skipLimits advances past a WASM limits encoding.
func skipLimits(data []byte, pos int) int {
	flag := data[pos]
	pos++
	_, n := readULEB128(data[pos:]) // min
	pos += n
	if flag == 0x01 {
		_, n = readULEB128(data[pos:]) // max
		pos += n
	}
	return pos
}

// parseTypes parses the type section into funcType structs.
func parseTypes(data []byte) ([]funcType, error) {
	pos := 0
	count, n := readULEB128(data[pos:])
	pos += n

	types := make([]funcType, 0, count)
	for i := uint32(0); i < count; i++ {
		if data[pos] != wasmFuncTy {
			return nil, fmt.Errorf("expected functype 0x60, got 0x%02x", data[pos])
		}
		pos++

		paramCount, n := readULEB128(data[pos:])
		pos += n
		params := make([]byte, paramCount)
		copy(params, data[pos:pos+int(paramCount)])
		pos += int(paramCount)

		resultCount, n := readULEB128(data[pos:])
		pos += n
		results := make([]byte, resultCount)
		copy(results, data[pos:pos+int(resultCount)])
		pos += int(resultCount)

		types = append(types, funcType{params: params, results: results})
	}

	return types, nil
}

// parseFuncSection returns the type indices from the function section.
func parseFuncSection(data []byte) ([]uint32, error) {
	pos := 0
	count, n := readULEB128(data[pos:])
	pos += n

	indices := make([]uint32, 0, count)
	for i := uint32(0); i < count; i++ {
		idx, n := readULEB128(data[pos:])
		pos += n
		indices = append(indices, idx)
	}

	return indices, nil
}

func typesEqual(a, b funcType) bool {
	if len(a.params) != len(b.params) || len(a.results) != len(b.results) {
		return false
	}
	for i := range a.params {
		if a.params[i] != b.params[i] {
			return false
		}
	}
	for i := range a.results {
		if a.results[i] != b.results[i] {
			return false
		}
	}
	return true
}

// buildStubs creates the stub function types, type indices (relative to stubTypes),
// code bodies, and global definitions for the WASI stubs.
// It matches each WASI import by name to its known stub implementation.
func buildStubs(wasiImports []wasiImportInfo, existingTypes []funcType) (
	stubTypes []funcType,
	stubTypeIndices []uint32, // indices into stubTypes
	stubBodies [][]byte,
	stubGlobals [][]byte,
	err error,
) {
	// The stub functions need globals for clock_time and prng_state.
	// These are appended to the global section.
	// We use placeholder global indices (0, 1) in the body builders;
	// the caller will remap them to (existingGlobalCount+0, existingGlobalCount+1).
	needClockGlobal := false
	needPrngGlobal := false

	// Map of known stub signatures and builders.
	type stubDef struct {
		sig     funcType
		builder func() []byte
	}

	defs := map[string]stubDef{
		"args_get":            {funcType{[]byte{wasmI32, wasmI32}, []byte{wasmI32}}, buildArgsGet},
		"args_sizes_get":      {funcType{[]byte{wasmI32, wasmI32}, []byte{wasmI32}}, buildArgsSizesGet},
		"clock_time_get":      {funcType{[]byte{wasmI32, wasmI64, wasmI32}, []byte{wasmI32}}, buildClockTimeGet},
		"environ_get":         {funcType{[]byte{wasmI32, wasmI32}, []byte{wasmI32}}, buildEnvironGet},
		"environ_sizes_get":   {funcType{[]byte{wasmI32, wasmI32}, []byte{wasmI32}}, buildEnvironSizesGet},
		"fd_write":            {funcType{[]byte{wasmI32, wasmI32, wasmI32, wasmI32}, []byte{wasmI32}}, buildFdWrite},
		"random_get":          {funcType{[]byte{wasmI32, wasmI32}, []byte{wasmI32}}, buildRandomGet},
		"poll_oneoff":         {funcType{[]byte{wasmI32, wasmI32, wasmI32, wasmI32}, []byte{wasmI32}}, buildPollOneoff},
		"proc_exit":           {funcType{[]byte{wasmI32}, nil}, buildProcExit},
		"sched_yield":         {funcType{nil, []byte{wasmI32}}, buildSchedYield},
		"fd_close":            {funcType{[]byte{wasmI32}, []byte{wasmI32}}, buildFdClose},
		"fd_fdstat_get":       {funcType{[]byte{wasmI32, wasmI32}, []byte{wasmI32}}, buildFdFdstatGet},
		"fd_fdstat_set_flags": {funcType{[]byte{wasmI32, wasmI32}, []byte{wasmI32}}, buildFdFdstatSetFlags},
		"fd_prestat_get":      {funcType{[]byte{wasmI32, wasmI32}, []byte{wasmI32}}, buildFdPrestatGet},
		"fd_prestat_dir_name": {funcType{[]byte{wasmI32, wasmI32, wasmI32}, []byte{wasmI32}}, buildFdPrestatDirName},
	}

	// Collect unique stub types.
	typeMap := make(map[string]int) // key=sig fingerprint -> index in stubTypes

	for _, wi := range wasiImports {
		def, ok := defs[wi.name]
		if !ok {
			// Unknown WASI function — generate a trap stub matching the import's type signature.
			// Look up the type from existingTypes using the import's typeIdx.
			if int(wi.typeIdx) >= len(existingTypes) {
				return nil, nil, nil, nil, fmt.Errorf("WASI import %q: type index %d out of range", wi.name, wi.typeIdx)
			}
			sig := existingTypes[wi.typeIdx]
			body := buildTrapStub(sig)
			key := typeFingerprint(sig)
			stIdx, exists := typeMap[key]
			if !exists {
				stIdx = len(stubTypes)
				stubTypes = append(stubTypes, sig)
				typeMap[key] = stIdx
			}
			stubTypeIndices = append(stubTypeIndices, uint32(stIdx))
			stubBodies = append(stubBodies, body)
			continue
		}

		key := typeFingerprint(def.sig)
		stIdx, exists := typeMap[key]
		if !exists {
			stIdx = len(stubTypes)
			stubTypes = append(stubTypes, def.sig)
			typeMap[key] = stIdx
		}
		stubTypeIndices = append(stubTypeIndices, uint32(stIdx))
		stubBodies = append(stubBodies, def.builder())

		if wi.name == "clock_time_get" {
			needClockGlobal = true
		}
		if wi.name == "random_get" {
			needPrngGlobal = true
		}
	}

	// Build globals.
	if needClockGlobal {
		var g []byte
		g = append(g, wasmI64, 0x01) // i64, mutable
		g = append(g, opI64Const)
		g = appendSLEB128(g, 1700000000000000000)
		g = append(g, opEnd)
		stubGlobals = append(stubGlobals, g)
	}
	if needPrngGlobal {
		var g []byte
		g = append(g, wasmI64, 0x01) // i64, mutable
		g = append(g, opI64Const)
		g = appendSLEB128(g, 88172645463325252)
		g = append(g, opEnd)
		stubGlobals = append(stubGlobals, g)
	}

	return stubTypes, stubTypeIndices, stubBodies, stubGlobals, nil
}

// typeFingerprint returns a unique string key for a funcType.
func typeFingerprint(ft funcType) string {
	return string(ft.params) + "|" + string(ft.results)
}

// buildTrapStub builds a function body that immediately traps (unreachable).
func buildTrapStub(sig funcType) []byte {
	var code []byte
	code = append(code, opUnreachable)
	return codeBody(noLocals(), code)
}

// appendRebuiltTypeSection encodes the full type section.
func appendRebuiltTypeSection(result []byte, types []funcType) []byte {
	var sec []byte
	sec = appendULEB128(sec, uint32(len(types)))
	for _, t := range types {
		sec = append(sec, wasmFuncTy)
		sec = appendULEB128(sec, uint32(len(t.params)))
		sec = append(sec, t.params...)
		sec = appendULEB128(sec, uint32(len(t.results)))
		sec = append(sec, t.results...)
	}
	result = append(result, sectionType)
	result = appendULEB128(result, uint32(len(sec)))
	result = append(result, sec...)
	return result
}

// appendRebuiltImportSection encodes the import section without WASI imports.
func appendRebuiltImportSection(result []byte, imports []importEntry) []byte {
	var sec []byte
	sec = appendULEB128(sec, uint32(len(imports)))
	for _, imp := range imports {
		sec = appendString(sec, imp.module)
		sec = appendString(sec, imp.name)
		sec = append(sec, imp.kind)
		if imp.kind == 0x00 {
			sec = appendULEB128(sec, imp.typeIdx)
		} else {
			sec = append(sec, imp.rawDesc...)
		}
	}
	result = append(result, sectionImport)
	result = appendULEB128(result, uint32(len(sec)))
	result = append(result, sec...)
	return result
}

// appendRebuiltFuncSection encodes the function section with stubs appended.
func appendRebuiltFuncSection(result []byte, localTypeIndices, stubTypeIndices []uint32) []byte {
	total := len(localTypeIndices) + len(stubTypeIndices)
	var sec []byte
	sec = appendULEB128(sec, uint32(total))
	for _, idx := range localTypeIndices {
		sec = appendULEB128(sec, idx)
	}
	for _, idx := range stubTypeIndices {
		sec = appendULEB128(sec, idx)
	}
	result = append(result, sectionFunc)
	result = appendULEB128(result, uint32(len(sec)))
	result = append(result, sec...)
	return result
}

// appendRebuiltGlobalSection appends stub globals to the existing global section.
func appendRebuiltGlobalSection(result []byte, existingData []byte, stubGlobals [][]byte) []byte {
	// Parse existing count.
	pos := 0
	existingCount, n := readULEB128(existingData[pos:])
	pos += n
	existingRest := existingData[pos:]

	newCount := existingCount + uint32(len(stubGlobals))
	var sec []byte
	sec = appendULEB128(sec, newCount)
	sec = append(sec, existingRest...)
	for _, g := range stubGlobals {
		sec = append(sec, g...)
	}

	result = append(result, sectionGlobal)
	result = appendULEB128(result, uint32(len(sec)))
	result = append(result, sec...)
	return result
}

// rebuildExportSectionWithRemap rebuilds the export section, remapping function
// indices and renaming _start to _initialize.
func rebuildExportSectionWithRemap(data []byte, funcRemap map[uint32]uint32) ([]byte, error) {
	pos := 0
	count, n := readULEB128(data[pos:])
	pos += n

	type wasmExport struct {
		name  string
		kind  byte
		index uint32
	}

	exports := make([]wasmExport, 0, count)
	for i := uint32(0); i < count; i++ {
		nameLen, n := readULEB128(data[pos:])
		pos += n
		name := string(data[pos : pos+int(nameLen)])
		pos += int(nameLen)

		kind := data[pos]
		pos++

		index, n := readULEB128(data[pos:])
		pos += n

		// Remap function export indices.
		if kind == 0x00 { // function export
			if newIdx, ok := funcRemap[index]; ok {
				index = newIdx
			}
		}

		// Rename _start to _initialize.
		if name == "_start" && kind == 0x00 {
			name = "_initialize"
		}

		exports = append(exports, wasmExport{name: name, kind: kind, index: index})
	}

	var out []byte
	out = appendULEB128(out, uint32(len(exports)))
	for _, e := range exports {
		out = appendString(out, e.name)
		out = append(out, e.kind)
		out = appendULEB128(out, e.index)
	}

	return out, nil
}

// appendRemappedStartSection remaps the start function index.
func appendRemappedStartSection(result []byte, data []byte, funcRemap map[uint32]uint32) []byte {
	idx, _ := readULEB128(data)
	if newIdx, ok := funcRemap[idx]; ok {
		idx = newIdx
	}
	var sec []byte
	sec = appendULEB128(sec, idx)
	result = append(result, sectionStart)
	result = appendULEB128(result, uint32(len(sec)))
	result = append(result, sec...)
	return result
}

// remapElementSection rewrites all function indices in the element section.
func remapElementSection(data []byte, funcRemap map[uint32]uint32) ([]byte, error) {
	pos := 0
	segCount, n := readULEB128(data[pos:])
	pos += n

	var out []byte
	out = appendULEB128(out, segCount)

	for seg := uint32(0); seg < segCount; seg++ {
		if pos >= len(data) {
			return nil, fmt.Errorf("element section truncated at segment %d", seg)
		}

		flags, n := readULEB128(data[pos:])
		pos += n
		out = appendULEB128(out, flags)

		switch flags {
		case 0:
			// Active: expr offset, vec<funcidx>
			// Copy init expr until 0x0b (end).
			exprData, exprLen := copyExpr(data[pos:])
			out = append(out, exprData...)
			pos += exprLen

			funcCount, n := readULEB128(data[pos:])
			pos += n
			out = appendULEB128(out, funcCount)
			for i := uint32(0); i < funcCount; i++ {
				idx, n := readULEB128(data[pos:])
				pos += n
				if newIdx, ok := funcRemap[idx]; ok {
					idx = newIdx
				}
				out = appendULEB128(out, idx)
			}

		case 1:
			// Passive: elemkind, vec<funcidx>
			elemKind := data[pos]
			pos++
			out = append(out, elemKind)

			funcCount, n := readULEB128(data[pos:])
			pos += n
			out = appendULEB128(out, funcCount)
			for i := uint32(0); i < funcCount; i++ {
				idx, n := readULEB128(data[pos:])
				pos += n
				if newIdx, ok := funcRemap[idx]; ok {
					idx = newIdx
				}
				out = appendULEB128(out, idx)
			}

		case 2:
			// Active with tableidx: tableidx, expr, elemkind, vec<funcidx>
			tableIdx, n := readULEB128(data[pos:])
			pos += n
			out = appendULEB128(out, tableIdx)

			exprData, exprLen := copyExpr(data[pos:])
			out = append(out, exprData...)
			pos += exprLen

			elemKind := data[pos]
			pos++
			out = append(out, elemKind)

			funcCount, n := readULEB128(data[pos:])
			pos += n
			out = appendULEB128(out, funcCount)
			for i := uint32(0); i < funcCount; i++ {
				idx, n := readULEB128(data[pos:])
				pos += n
				if newIdx, ok := funcRemap[idx]; ok {
					idx = newIdx
				}
				out = appendULEB128(out, idx)
			}

		case 3:
			// Declarative: elemkind, vec<funcidx>
			elemKind := data[pos]
			pos++
			out = append(out, elemKind)

			funcCount, n := readULEB128(data[pos:])
			pos += n
			out = appendULEB128(out, funcCount)
			for i := uint32(0); i < funcCount; i++ {
				idx, n := readULEB128(data[pos:])
				pos += n
				if newIdx, ok := funcRemap[idx]; ok {
					idx = newIdx
				}
				out = appendULEB128(out, idx)
			}

		case 4:
			// Active: expr offset, vec<expr>
			exprData, exprLen := copyExpr(data[pos:])
			out = append(out, exprData...)
			pos += exprLen

			exprCount, n := readULEB128(data[pos:])
			pos += n
			out = appendULEB128(out, exprCount)
			for i := uint32(0); i < exprCount; i++ {
				remapped, consumed := remapExpr(data[pos:], funcRemap)
				out = append(out, remapped...)
				pos += consumed
			}

		case 5:
			// Passive with reftype: reftype, vec<expr>
			refType := data[pos]
			pos++
			out = append(out, refType)

			exprCount, n := readULEB128(data[pos:])
			pos += n
			out = appendULEB128(out, exprCount)
			for i := uint32(0); i < exprCount; i++ {
				remapped, consumed := remapExpr(data[pos:], funcRemap)
				out = append(out, remapped...)
				pos += consumed
			}

		case 6:
			// Active with tableidx, reftype: tableidx, expr, reftype, vec<expr>
			tableIdx, n := readULEB128(data[pos:])
			pos += n
			out = appendULEB128(out, tableIdx)

			exprData, exprLen := copyExpr(data[pos:])
			out = append(out, exprData...)
			pos += exprLen

			refType := data[pos]
			pos++
			out = append(out, refType)

			exprCount, n := readULEB128(data[pos:])
			pos += n
			out = appendULEB128(out, exprCount)
			for i := uint32(0); i < exprCount; i++ {
				remapped, consumed := remapExpr(data[pos:], funcRemap)
				out = append(out, remapped...)
				pos += consumed
			}

		case 7:
			// Declarative with reftype: reftype, vec<expr>
			refType := data[pos]
			pos++
			out = append(out, refType)

			exprCount, n := readULEB128(data[pos:])
			pos += n
			out = appendULEB128(out, exprCount)
			for i := uint32(0); i < exprCount; i++ {
				remapped, consumed := remapExpr(data[pos:], funcRemap)
				out = append(out, remapped...)
				pos += consumed
			}

		default:
			return nil, fmt.Errorf("unsupported element segment flags: %d", flags)
		}
	}

	return out, nil
}

// copyExpr copies a WASM constant expression (ending with 0x0b) and returns the bytes and length consumed.
func copyExpr(data []byte) ([]byte, int) {
	depth := 0
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case opBlock, opLoop, opIf:
			depth++
			// Skip blocktype (a single byte for void or valtype, or a sleb128 for typeidx)
		case opEnd:
			if depth == 0 {
				return data[:i+1], i + 1
			}
			depth--
		case opI32Const:
			_, n := readSLEB128(data[i+1:])
			i += n
		case opI64Const:
			_, n := readSLEB128(data[i+1:])
			i += n
		case opGlobalGet:
			_, n := readULEB128(data[i+1:])
			i += n
		case opRefFunc:
			_, n := readULEB128(data[i+1:])
			i += n
		}
	}
	// Shouldn't reach here for valid WASM.
	return data, len(data)
}

// remapExpr remaps function indices inside an init expression (ref.func references).
func remapExpr(data []byte, funcRemap map[uint32]uint32) ([]byte, int) {
	var out []byte
	i := 0
	for i < len(data) {
		op := data[i]
		switch op {
		case opEnd:
			out = append(out, op)
			return out, i + 1
		case opRefFunc:
			out = append(out, op)
			i++
			idx, n := readULEB128(data[i:])
			i += n
			if newIdx, ok := funcRemap[idx]; ok {
				idx = newIdx
			}
			out = appendULEB128(out, idx)
		case opI32Const:
			out = append(out, op)
			i++
			val, n := readSLEB128(data[i:])
			i += n
			out = appendSLEB128(out, val)
		case opI64Const:
			out = append(out, op)
			i++
			val, n := readSLEB128(data[i:])
			i += n
			out = appendSLEB128(out, val)
		case opGlobalGet:
			out = append(out, op)
			i++
			idx, n := readULEB128(data[i:])
			i += n
			out = appendULEB128(out, idx)
		default:
			out = append(out, op)
			i++
		}
	}
	return out, i
}

// readSLEB128 decodes a signed LEB128 value.
func readSLEB128(data []byte) (int64, int) {
	var result int64
	var shift uint
	var i int
	for i = 0; i < len(data); i++ {
		b := data[i]
		result |= int64(b&0x7f) << shift
		shift += 7
		if b&0x80 == 0 {
			if shift < 64 && b&0x40 != 0 {
				result |= -(1 << shift)
			}
			return result, i + 1
		}
	}
	return result, i
}

// rebuildCodeSection remaps function indices in existing code bodies and appends stub bodies.
func rebuildCodeSection(data []byte, funcRemap map[uint32]uint32, globalIdxBase uint32, stubBodies [][]byte) ([]byte, error) {
	pos := 0
	bodyCount, n := readULEB128(data[pos:])
	pos += n

	newCount := bodyCount + uint32(len(stubBodies))
	var out []byte
	out = appendULEB128(out, newCount)

	// Remap existing function bodies.
	for i := uint32(0); i < bodyCount; i++ {
		bodySize, n := readULEB128(data[pos:])
		pos += n
		bodyData := data[pos : pos+int(bodySize)]
		pos += int(bodySize)

		remappedBody := remapCodeBody(bodyData, funcRemap)
		out = appendULEB128(out, uint32(len(remappedBody)))
		out = append(out, remappedBody...)
	}

	// Append stub function bodies with global index remapping.
	for _, body := range stubBodies {
		// Remap global indices in stub bodies: global 0 -> globalIdxBase, global 1 -> globalIdxBase+1.
		remappedBody := remapStubGlobals(body, globalIdxBase)
		out = appendULEB128(out, uint32(len(remappedBody)))
		out = append(out, remappedBody...)
	}

	return out, nil
}

// remapCodeBody scans a function body and remaps call and ref.func indices.
// It uses a two-pass approach: first finds patch locations, then applies them.
// This preserves the original byte encoding (including non-canonical LEB128)
// for all instructions except the patched call/ref.func operands.
func remapCodeBody(body []byte, funcRemap map[uint32]uint32) []byte {
	// Find all call and ref.func instruction positions and their operand ranges.
	type patch struct {
		opStart int    // position of operand start (after opcode byte)
		opEnd   int    // position after operand
		oldIdx  uint32 // original function index
		newIdx  uint32 // remapped function index
	}

	var patches []patch

	// Skip past locals declarations.
	pos := 0
	localDeclCount, n := readULEB128(body[pos:])
	pos += n
	for i := uint32(0); i < localDeclCount; i++ {
		_, n = readULEB128(body[pos:]) // count
		pos += n
		pos++ // type
	}

	// Scan instructions to find call/ref.func locations.
	for pos < len(body) {
		op := body[pos]
		pos++

		switch op {
		case opCall, opRefFunc:
			opStart := pos
			idx, n := readULEB128(body[pos:])
			pos += n
			if newIdx, ok := funcRemap[idx]; ok && newIdx != idx {
				patches = append(patches, patch{opStart: opStart, opEnd: pos, oldIdx: idx, newIdx: newIdx})
			}

		case opCallIndirect:
			_, n := readULEB128(body[pos:])
			pos += n // type index
			_, n = readULEB128(body[pos:])
			pos += n // table index

		case opBlock, opLoop, opIf:
			// blocktype: 0x40 (void), valtype, or s33 type index
			bt := body[pos]
			if bt == 0x40 || (bt >= 0x6f && bt <= 0x7f) {
				pos++
			} else {
				_, n := readSLEB128(body[pos:])
				pos += n
			}

		case opBr, opBrIf:
			_, n := readULEB128(body[pos:])
			pos += n

		case 0x0e: // br_table
			labelCount, n := readULEB128(body[pos:])
			pos += n
			for i := uint32(0); i <= labelCount; i++ {
				_, n = readULEB128(body[pos:])
				pos += n
			}

		case opLocalGet, opLocalSet, 0x22: // local.get/set/tee
			_, n := readULEB128(body[pos:])
			pos += n

		case opGlobalGet, opGlobalSet:
			_, n := readULEB128(body[pos:])
			pos += n

		case 0x25, 0x26: // table.get/set
			_, n := readULEB128(body[pos:])
			pos += n

		case opI32Const:
			_, n := readSLEB128(body[pos:])
			pos += n

		case opI64Const:
			_, n := readSLEB128(body[pos:])
			pos += n

		case 0x43: // f32.const
			pos += 4

		case 0x44: // f64.const
			pos += 8

		case 0x1c: // select t*
			count, n := readULEB128(body[pos:])
			pos += n
			pos += int(count)

		case 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f,
			0x30, 0x31, 0x32, 0x33, 0x34, 0x35,
			0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e:
			_, n := readULEB128(body[pos:])
			pos += n // align
			_, n = readULEB128(body[pos:])
			pos += n // offset

		case 0x3f, 0x40: // memory.size, memory.grow
			_, n := readULEB128(body[pos:])
			pos += n

		case 0xfc: // multi-byte prefix
			subOp, n := readULEB128(body[pos:])
			pos += n
			switch subOp {
			case 8: // memory.init
				_, n = readULEB128(body[pos:])
				pos += n
				_, n = readULEB128(body[pos:])
				pos += n
			case 9: // data.drop
				_, n = readULEB128(body[pos:])
				pos += n
			case 10: // memory.copy
				_, n = readULEB128(body[pos:])
				pos += n
				_, n = readULEB128(body[pos:])
				pos += n
			case 11: // memory.fill
				_, n = readULEB128(body[pos:])
				pos += n
			case 12: // table.init
				_, n = readULEB128(body[pos:])
				pos += n
				_, n = readULEB128(body[pos:])
				pos += n
			case 13: // elem.drop
				_, n = readULEB128(body[pos:])
				pos += n
			case 14: // table.copy
				_, n = readULEB128(body[pos:])
				pos += n
				_, n = readULEB128(body[pos:])
				pos += n
			case 15, 16, 17: // table.grow/size/fill
				_, n = readULEB128(body[pos:])
				pos += n
			}

		case 0xfd: // SIMD prefix
			subOp, n := readULEB128(body[pos:])
			pos += n
			if subOp <= 11 || (subOp >= 92 && subOp <= 93) {
				_, n = readULEB128(body[pos:])
				pos += n // align
				_, n = readULEB128(body[pos:])
				pos += n // offset
			} else if subOp == 12 || subOp == 13 {
				pos += 16
			} else if subOp >= 21 && subOp <= 34 {
				pos++ // laneidx
			} else if subOp >= 84 && subOp <= 91 {
				_, n = readULEB128(body[pos:])
				pos += n // align
				_, n = readULEB128(body[pos:])
				pos += n // offset
				pos++    // laneidx
			}

		case 0xfe: // atomic prefix
			subOp, n := readULEB128(body[pos:])
			pos += n
			if subOp == 3 {
				pos++ // fence byte
			} else {
				_, n = readULEB128(body[pos:])
				pos += n // align
				_, n = readULEB128(body[pos:])
				pos += n // offset
			}
		}
	}

	if len(patches) == 0 {
		return body
	}

	// Apply patches: copy body with call/ref.func operands replaced.
	var out []byte
	prevEnd := 0
	for _, p := range patches {
		// Copy everything before this patch verbatim.
		out = append(out, body[prevEnd:p.opStart]...)
		// Write the remapped index.
		out = appendULEB128(out, p.newIdx)
		prevEnd = p.opEnd
	}
	// Copy remaining bytes.
	out = append(out, body[prevEnd:]...)

	return out
}

// remapStubGlobals remaps global.get/global.set indices 0 and 1 in stub bodies
// to globalIdxBase+0 and globalIdxBase+1.
func remapStubGlobals(body []byte, globalIdxBase uint32) []byte {
	if globalIdxBase == 0 {
		return body // no remapping needed
	}

	// Skip past locals declarations.
	pos := 0
	localDeclCount, n := readULEB128(body[pos:])
	pos += n
	for i := uint32(0); i < localDeclCount; i++ {
		_, n = readULEB128(body[pos:]) // count
		pos += n
		pos++ // type
	}

	var out []byte
	out = append(out, body[:pos]...)

	for pos < len(body) {
		op := body[pos]
		pos++

		switch op {
		case opGlobalGet, opGlobalSet:
			out = append(out, op)
			idx, n := readULEB128(body[pos:])
			pos += n
			// Only remap small indices that belong to our stubs.
			if idx < 2 {
				idx += globalIdxBase
			}
			out = appendULEB128(out, idx)

		case opI32Const, opI64Const:
			out = append(out, op)
			val, n := readSLEB128(body[pos:])
			pos += n
			out = appendSLEB128(out, val)

		case opLocalGet, opLocalSet, 0x22: // local.tee
			out = append(out, op)
			idx, n := readULEB128(body[pos:])
			pos += n
			out = appendULEB128(out, idx)

		case 0x28, 0x29, 0x2a, 0x2b, 0x2c, 0x2d, 0x2e, 0x2f,
			0x30, 0x31, 0x32, 0x33, 0x34, 0x35,
			0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x3c, 0x3d, 0x3e:
			out = append(out, op)
			align, n := readULEB128(body[pos:])
			pos += n
			out = appendULEB128(out, align)
			offset, n := readULEB128(body[pos:])
			pos += n
			out = appendULEB128(out, offset)

		case opBlock, opLoop, opIf:
			out = append(out, op)
			bt := body[pos]
			if bt == 0x40 || bt == wasmI32 || bt == wasmI64 || bt == 0x7d || bt == 0x7c || bt == 0x70 || bt == 0x6f {
				out = append(out, bt)
				pos++
			} else {
				val, n := readSLEB128(body[pos:])
				pos += n
				out = appendSLEB128(out, val)
			}

		case opBr, opBrIf:
			out = append(out, op)
			idx, n := readULEB128(body[pos:])
			pos += n
			out = appendULEB128(out, idx)

		case 0x3f, 0x40: // memory.size, memory.grow
			out = append(out, op)
			idx, n := readULEB128(body[pos:])
			pos += n
			out = appendULEB128(out, idx)

		default:
			out = append(out, op)
		}
	}

	return out
}

// renameStartIfPresent renames _start to _initialize if found, returns wasm unchanged if not.
func renameStartIfPresent(wasm []byte) ([]byte, error) {
	patched, err := renameExport(wasm, "_start", "_initialize")
	if err != nil {
		// If _start not found, that's fine — module may already have _initialize.
		return wasm, nil
	}
	return patched, nil
}

// insertGlobalSection inserts a global section into a WASM binary that doesn't have one.
// It inserts after section 5 (memory) or before section 7 (export), wherever appropriate.
func insertGlobalSection(result []byte, globalData []byte) []byte {
	// Re-parse the result to find the right insertion point.
	if len(result) < 8 {
		return result
	}

	var newResult []byte
	newResult = append(newResult, result[:8]...) // magic + version
	pos := 8
	inserted := false

	for pos < len(result) {
		secID := result[pos]
		pos++
		secSize, n := readULEB128(result[pos:])
		pos += n
		secEnd := pos + int(secSize)

		// Insert global section before export (7), start (8), element (9), code (10), data (11), or datacount (12).
		if !inserted && secID >= sectionExport {
			newResult = append(newResult, sectionGlobal)
			newResult = appendULEB128(newResult, uint32(len(globalData)))
			newResult = append(newResult, globalData...)
			inserted = true
		}

		newResult = append(newResult, secID)
		newResult = appendULEB128(newResult, secSize)
		newResult = append(newResult, result[pos:secEnd]...)
		pos = secEnd
	}

	if !inserted {
		newResult = append(newResult, sectionGlobal)
		newResult = appendULEB128(newResult, uint32(len(globalData)))
		newResult = append(newResult, globalData...)
	}

	return newResult
}
