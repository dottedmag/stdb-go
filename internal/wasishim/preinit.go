package wasishim

import (
	"fmt"
	"strings"
)

// exportEntry represents a single export in the WASM export section.
type exportEntry struct {
	name  string
	kind  byte
	index uint32
}

// wrapPreinitWithInit modifies a WASM binary so that all __preinit__* exports
// call _initialize first (with a guard to only initialize once).
// This is needed for SpacetimeDB hosts that don't call _initialize before
// __preinit__* functions (e.g., stock v2.0.5).
func wrapPreinitWithInit(wasm []byte) ([]byte, error) {
	if len(wasm) < 8 || string(wasm[:4]) != "\x00asm" {
		return nil, fmt.Errorf("invalid WASM module")
	}

	sections, err := parseSections(wasm)
	if err != nil {
		return nil, err
	}

	exportSec := findSection(sections, sectionExport)
	if exportSec == nil {
		return wasm, nil
	}

	exports, err := parseExports(exportSec.data)
	if err != nil {
		return nil, fmt.Errorf("parse exports: %w", err)
	}

	// Find _initialize function index and __preinit__* exports.
	initFuncIdx := int64(-1)
	type preinitInfo struct {
		exportIdx int
		funcIdx   uint32
	}
	var preinits []preinitInfo

	for i, exp := range exports {
		if exp.kind != 0x00 {
			continue
		}
		if exp.name == "_initialize" {
			initFuncIdx = int64(exp.index)
		}
		if strings.HasPrefix(exp.name, "__preinit__") {
			preinits = append(preinits, preinitInfo{exportIdx: i, funcIdx: exp.index})
		}
	}

	if initFuncIdx < 0 || len(preinits) == 0 {
		return wasm, nil
	}

	// Parse type section to find or add void->void type.
	typeSec := findSection(sections, sectionType)
	if typeSec == nil {
		return wasm, nil
	}
	existingTypes, err := parseTypes(typeSec.data)
	if err != nil {
		return nil, fmt.Errorf("parse types: %w", err)
	}

	voidVoid := funcType{params: nil, results: nil}
	voidVoidIdx := -1
	for i, t := range existingTypes {
		if typesEqual(t, voidVoid) {
			voidVoidIdx = i
			break
		}
	}
	if voidVoidIdx < 0 {
		voidVoidIdx = len(existingTypes)
		existingTypes = append(existingTypes, voidVoid)
	}

	// Count total existing functions (imports + locals).
	importSec := findSection(sections, sectionImport)
	importFuncCount := uint32(0)
	if importSec != nil {
		imps, err := parseImports(importSec.data)
		if err != nil {
			return nil, fmt.Errorf("parse imports: %w", err)
		}
		for _, imp := range imps {
			if imp.kind == 0x00 {
				importFuncCount++
			}
		}
	}

	funcSec := findSection(sections, sectionFunc)
	if funcSec == nil {
		return wasm, nil
	}
	localFuncTypeIndices, err := parseFuncSection(funcSec.data)
	if err != nil {
		return nil, fmt.Errorf("parse func section: %w", err)
	}

	totalExistingFuncs := importFuncCount + uint32(len(localFuncTypeIndices))

	// Count existing globals for the new initialized flag.
	globalSec := findSection(sections, sectionGlobal)
	existingGlobalCount := uint32(0)
	if globalSec != nil {
		existingGlobalCount, _ = readULEB128(globalSec.data)
	}
	initializedGlobalIdx := existingGlobalCount

	// Build wrapper functions and update exports.
	wrapperStartIdx := totalExistingFuncs
	wrapperTypeIndices := make([]uint32, len(preinits))
	wrapperBodies := make([][]byte, len(preinits))

	for i, pi := range preinits {
		wrapperTypeIndices[i] = uint32(voidVoidIdx)
		wrapperBodies[i] = buildPreinitWrapper(initializedGlobalIdx, uint32(initFuncIdx), pi.funcIdx)
		exports[pi.exportIdx].index = wrapperStartIdx + uint32(i)
	}

	// Build the initialized global (i32, mut, init 0).
	initGlobal := buildInitializedGlobal()

	// Rebuild module section by section.
	var result []byte
	result = append(result, wasm[:8]...) // magic + version

	for _, sec := range sections {
		switch sec.id {
		case sectionType:
			result = appendRebuiltTypeSection(result, existingTypes)

		case sectionFunc:
			result = appendRebuiltFuncSection(result, localFuncTypeIndices, wrapperTypeIndices)

		case sectionGlobal:
			result = appendRebuiltGlobalSection(result, sec.data, [][]byte{initGlobal})

		case sectionExport:
			result = appendRebuiltExportSection(result, exports)

		case sectionCode:
			result = appendCodeSectionWithWrappers(result, sec.data, wrapperBodies)

		default:
			result = append(result, sec.id)
			result = appendULEB128(result, uint32(len(sec.data)))
			result = append(result, sec.data...)
		}
	}

	// If no global section existed, add one.
	if globalSec == nil {
		var gSec []byte
		gSec = appendULEB128(gSec, 1)
		gSec = append(gSec, initGlobal...)
		result = insertGlobalSection(result, gSec)
	}

	return result, nil
}

// parseExports parses the export section data into exportEntry structs.
func parseExports(data []byte) ([]exportEntry, error) {
	pos := 0
	count, n := readULEB128(data[pos:])
	pos += n

	entries := make([]exportEntry, 0, count)
	for i := uint32(0); i < count; i++ {
		nameLen, n := readULEB128(data[pos:])
		pos += n
		name := string(data[pos : pos+int(nameLen)])
		pos += int(nameLen)

		kind := data[pos]
		pos++

		index, n := readULEB128(data[pos:])
		pos += n

		entries = append(entries, exportEntry{name: name, kind: kind, index: index})
	}

	return entries, nil
}

// appendRebuiltExportSection encodes the export section from parsed entries.
func appendRebuiltExportSection(result []byte, exports []exportEntry) []byte {
	var sec []byte
	sec = appendULEB128(sec, uint32(len(exports)))
	for _, exp := range exports {
		sec = appendString(sec, exp.name)
		sec = append(sec, exp.kind)
		sec = appendULEB128(sec, exp.index)
	}
	result = append(result, sectionExport)
	result = appendULEB128(result, uint32(len(sec)))
	result = append(result, sec...)
	return result
}

// appendCodeSectionWithWrappers copies the existing code section and appends
// wrapper function bodies at the end.
func appendCodeSectionWithWrappers(result []byte, existingCodeData []byte, wrapperBodies [][]byte) []byte {
	pos := 0
	bodyCount, n := readULEB128(existingCodeData[pos:])
	pos += n

	newCount := bodyCount + uint32(len(wrapperBodies))
	var sec []byte
	sec = appendULEB128(sec, newCount)

	// Copy existing bodies as-is (they are already correctly encoded).
	sec = append(sec, existingCodeData[pos:]...)

	// Append wrapper bodies.
	for _, body := range wrapperBodies {
		sec = appendULEB128(sec, uint32(len(body)))
		sec = append(sec, body...)
	}

	result = append(result, sectionCode)
	result = appendULEB128(result, uint32(len(sec)))
	result = append(result, sec...)
	return result
}

// buildPreinitWrapper creates a function body that calls _initialize (once)
// then delegates to the original __preinit__* function.
//
// Pseudocode:
//
//	if !initialized { initialized = 1; _initialize(); }
//	original();
func buildPreinitWrapper(initializedGlobalIdx, initFuncIdx, originalFuncIdx uint32) []byte {
	var code []byte

	// global.get $initialized; i32.eqz; if void
	code = append(code, opGlobalGet)
	code = appendULEB128(code, initializedGlobalIdx)
	code = append(code, 0x45) // i32.eqz
	code = append(code, opIf, opBlockVoid)

	// initialized = 1
	code = append(code, i32Const(1)...)
	code = append(code, opGlobalSet)
	code = appendULEB128(code, initializedGlobalIdx)

	// call _initialize
	code = append(code, opCall)
	code = appendULEB128(code, initFuncIdx)

	code = append(code, opEnd) // end if

	// call original preinit function
	code = append(code, opCall)
	code = appendULEB128(code, originalFuncIdx)

	return codeBody(noLocals(), code)
}

// buildInitializedGlobal creates a mutable i32 global initialized to 0.
func buildInitializedGlobal() []byte {
	var g []byte
	g = append(g, wasmI32, 0x01) // i32, mutable
	g = append(g, opI32Const)
	g = appendSLEB128(g, 0)
	g = append(g, opEnd)
	return g
}
