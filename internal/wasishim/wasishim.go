// Package wasishim rewrites Go WASM modules to replace wasi_snapshot_preview1
// imports with local stub functions, making the module self-contained.
//
// Go targeting wasip1/wasm emits imports from wasi_snapshot_preview1 that
// SpacetimeDB hosts do not provide. Rather than using wasm-merge (which can
// corrupt Go modules), this package directly rewrites the WASM binary to
// replace those imports with local stub implementations.
//
// The stub behavior matches SpacetimeDB's Rust wasi_stubs.rs.
package wasishim

import (
	"fmt"
)

// renameExport rewrites a WASM binary, changing the export named oldName to newName.
// It reconstructs the export section since name lengths may differ.
func renameExport(wasm []byte, oldName, newName string) ([]byte, error) {
	if len(wasm) < 8 {
		return nil, fmt.Errorf("invalid WASM module")
	}

	// Verify magic and version.
	if string(wasm[:4]) != "\x00asm" {
		return nil, fmt.Errorf("invalid WASM magic")
	}

	var result []byte
	result = append(result, wasm[:8]...) // magic + version

	pos := 8
	found := false

	for pos < len(wasm) {
		if pos >= len(wasm) {
			break
		}
		sectionID := wasm[pos]
		pos++

		sectionLen, n := readULEB128(wasm[pos:])
		pos += n
		sectionEnd := pos + int(sectionLen)

		if sectionID != sectionExport {
			// Copy non-export sections as-is.
			result = append(result, sectionID)
			result = appendULEB128(result, sectionLen)
			result = append(result, wasm[pos:sectionEnd]...)
			pos = sectionEnd
			continue
		}

		// Rebuild the export section with the renamed export.
		sectionData := wasm[pos:sectionEnd]
		newSection, renamed, err := rebuildExportSection(sectionData, oldName, newName)
		if err != nil {
			return nil, err
		}
		found = renamed

		result = append(result, sectionID)
		result = appendULEB128(result, uint32(len(newSection)))
		result = append(result, newSection...)
		pos = sectionEnd
	}

	if !found {
		return nil, fmt.Errorf("export %q not found", oldName)
	}

	return result, nil
}

// rebuildExportSection parses an export section and rebuilds it with oldName → newName.
func rebuildExportSection(data []byte, oldName, newName string) ([]byte, bool, error) {
	pos := 0
	count, n := readULEB128(data[pos:])
	pos += n

	type wasmExport struct {
		name  string
		kind  byte
		index uint32
	}

	exports := make([]wasmExport, count)
	renamed := false

	for i := uint32(0); i < count; i++ {
		nameLen, n := readULEB128(data[pos:])
		pos += n
		name := string(data[pos : pos+int(nameLen)])
		pos += int(nameLen)

		kind := data[pos]
		pos++

		index, n := readULEB128(data[pos:])
		pos += n

		if name == oldName {
			name = newName
			renamed = true
		}

		exports[i] = wasmExport{name: name, kind: kind, index: index}
	}

	// Rebuild section content.
	var out []byte
	out = appendULEB128(out, count)
	for _, e := range exports {
		out = appendString(out, e.name)
		out = append(out, e.kind)
		out = appendULEB128(out, e.index)
	}

	return out, renamed, nil
}

// readULEB128 decodes an unsigned LEB128 value, returning the value and bytes consumed.
func readULEB128(data []byte) (uint32, int) {
	var result uint32
	var shift uint
	for i, b := range data {
		result |= uint32(b&0x7f) << shift
		if b&0x80 == 0 {
			return result, i + 1
		}
		shift += 7
	}
	return result, len(data)
}
