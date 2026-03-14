package wasishim_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/wasishim"
)

// buildTestModuleWithWASIImports constructs a minimal WASM module that imports
// several wasi_snapshot_preview1 functions plus has one local function.
// This simulates what Go's wasip1/wasm target produces.
func buildTestModuleWithWASIImports() []byte {
	var buf []byte

	// WASM magic + version
	buf = append(buf, 0x00, 0x61, 0x73, 0x6d) // magic
	buf = append(buf, 0x01, 0x00, 0x00, 0x00) // version 1

	// Type section: define function types
	// type 0: () -> ()
	// type 1: (i32, i32) -> i32
	// type 2: (i32) -> ()
	var typeSec []byte
	typeSec = appendTestULEB128(typeSec, 3) // 3 types

	// type 0: () -> ()
	typeSec = append(typeSec, 0x60) // functype
	typeSec = appendTestULEB128(typeSec, 0)
	typeSec = appendTestULEB128(typeSec, 0)

	// type 1: (i32, i32) -> i32
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 2)
	typeSec = append(typeSec, 0x7f, 0x7f) // i32, i32
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x7f) // i32

	// type 2: (i32) -> ()
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x7f) // i32
	typeSec = appendTestULEB128(typeSec, 0)

	buf = appendTestSection(buf, 1, typeSec)

	// Import section: 3 WASI imports + 1 memory import
	var importSec []byte
	importSec = appendTestULEB128(importSec, 4) // 4 imports

	// import 0: wasi_snapshot_preview1.args_get (func, type 1) -> func idx 0
	importSec = appendTestString(importSec, "wasi_snapshot_preview1")
	importSec = appendTestString(importSec, "args_get")
	importSec = append(importSec, 0x00) // func
	importSec = appendTestULEB128(importSec, 1)

	// import 1: wasi_snapshot_preview1.args_sizes_get (func, type 1) -> func idx 1
	importSec = appendTestString(importSec, "wasi_snapshot_preview1")
	importSec = appendTestString(importSec, "args_sizes_get")
	importSec = append(importSec, 0x00) // func
	importSec = appendTestULEB128(importSec, 1)

	// import 2: wasi_snapshot_preview1.proc_exit (func, type 2) -> func idx 2
	importSec = appendTestString(importSec, "wasi_snapshot_preview1")
	importSec = appendTestString(importSec, "proc_exit")
	importSec = append(importSec, 0x00) // func
	importSec = appendTestULEB128(importSec, 2)

	// import 3: env.memory (memory, min=1)
	importSec = appendTestString(importSec, "env")
	importSec = appendTestString(importSec, "memory")
	importSec = append(importSec, 0x02) // memory
	importSec = append(importSec, 0x00) // no max
	importSec = appendTestULEB128(importSec, 1)

	buf = appendTestSection(buf, 2, importSec)

	// Function section: 2 local functions
	var funcSec []byte
	funcSec = appendTestULEB128(funcSec, 2) // 2 functions
	funcSec = appendTestULEB128(funcSec, 0) // func 3 (after 3 imports): type 0
	funcSec = appendTestULEB128(funcSec, 0) // func 4: type 0

	buf = appendTestSection(buf, 3, funcSec)

	// Export section: export func 3 as "_start", func 4 as "reducer"
	var exportSec []byte
	exportSec = appendTestULEB128(exportSec, 2) // 2 exports

	exportSec = appendTestString(exportSec, "_start")
	exportSec = append(exportSec, 0x00) // func
	exportSec = appendTestULEB128(exportSec, 3)

	exportSec = appendTestString(exportSec, "reducer")
	exportSec = append(exportSec, 0x00) // func
	exportSec = appendTestULEB128(exportSec, 4)

	buf = appendTestSection(buf, 7, exportSec)

	// Code section: 2 function bodies
	var codeSec []byte
	codeSec = appendTestULEB128(codeSec, 2) // 2 bodies

	// func 3 body: calls func 0 (args_get with dummy args) then calls func 4
	var body0 []byte
	body0 = appendTestULEB128(body0, 0) // 0 locals
	body0 = append(body0, 0x10)         // call
	body0 = appendTestULEB128(body0, 4) // call func 4
	body0 = append(body0, 0x0b)         // end
	codeSec = appendTestULEB128(codeSec, uint32(len(body0)))
	codeSec = append(codeSec, body0...)

	// func 4 body: nop, end
	var body1 []byte
	body1 = appendTestULEB128(body1, 0) // 0 locals
	body1 = append(body1, 0x01)         // nop
	body1 = append(body1, 0x0b)         // end
	codeSec = appendTestULEB128(codeSec, uint32(len(body1)))
	codeSec = append(codeSec, body1...)

	buf = appendTestSection(buf, 10, codeSec)

	return buf
}

func appendTestULEB128(buf []byte, v uint32) []byte {
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if v == 0 {
			break
		}
	}
	return buf
}

func appendTestString(buf []byte, s string) []byte {
	buf = appendTestULEB128(buf, uint32(len(s)))
	buf = append(buf, s...)
	return buf
}

func appendTestSection(buf []byte, id byte, content []byte) []byte {
	buf = append(buf, id)
	buf = appendTestULEB128(buf, uint32(len(content)))
	buf = append(buf, content...)
	return buf
}

func TestRewriteWASIImportsRemovesWASI(t *testing.T) {
	input := buildTestModuleWithWASIImports()

	result, err := wasishim.RewriteWASIImportsForTest(input)
	require.NoError(t, err)

	// Verify WASM magic.
	require.True(t, len(result) > 8)
	assert.Equal(t, []byte{0x00, 0x61, 0x73, 0x6d}, result[:4])

	// Write to temp file and validate with wasm-opt if available.
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "rewritten.wasm")
	require.NoError(t, os.WriteFile(outPath, result, 0o644))

	wasmOpt, err := exec.LookPath("wasm-opt")
	if err != nil {
		t.Log("wasm-opt not available, skipping structural validation")
		return
	}

	cmd := exec.Command(wasmOpt, "--print", outPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "wasm-opt validation failed:\n%s", string(output))

	watOutput := string(output)

	// Verify no WASI imports remain.
	assert.NotContains(t, watOutput, "wasi_snapshot_preview1",
		"rewritten module should not contain wasi_snapshot_preview1 imports")

	// Verify _start was renamed to _initialize.
	assert.Contains(t, watOutput, "_initialize",
		"rewritten module should have _initialize export")
	assert.NotContains(t, watOutput, `"_start"`,
		"rewritten module should not have _start export")

	t.Logf("Rewritten module validated successfully (%d bytes)", len(result))
}

func TestRewriteWASIImportsPreservesNonWASI(t *testing.T) {
	input := buildTestModuleWithWASIImports()

	result, err := wasishim.RewriteWASIImportsForTest(input)
	require.NoError(t, err)

	// Write and check with wasm-opt.
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "rewritten.wasm")
	require.NoError(t, os.WriteFile(outPath, result, 0o644))

	wasmOpt, err := exec.LookPath("wasm-opt")
	if err != nil {
		t.Skip("wasm-opt not available")
	}

	cmd := exec.Command(wasmOpt, "--print", outPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "wasm-opt validation failed:\n%s", string(output))

	watOutput := string(output)

	// The env.memory import should be preserved.
	assert.Contains(t, watOutput, `"env"`,
		"rewritten module should preserve non-WASI imports")
	assert.Contains(t, watOutput, `"memory"`,
		"rewritten module should preserve memory import")

	// The "reducer" export should still be present.
	assert.Contains(t, watOutput, `"reducer"`,
		"rewritten module should preserve non-_start exports")
}

func TestRewriteWASIImportsCallRemapping(t *testing.T) {
	// This test verifies that call instructions are properly remapped.
	// In the test module, func 3 (local func 0) calls func 4 (local func 1).
	// After removing 3 WASI imports, the call target should shift.
	input := buildTestModuleWithWASIImports()

	result, err := wasishim.RewriteWASIImportsForTest(input)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "rewritten.wasm")
	require.NoError(t, os.WriteFile(outPath, result, 0o644))

	wasmOpt, err := exec.LookPath("wasm-opt")
	if err != nil {
		t.Skip("wasm-opt not available")
	}

	// Validate the module is structurally valid.
	cmd := exec.Command(wasmOpt, "--print", outPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "rewritten module is invalid:\n%s", string(output))
}

func TestRewriteWASINoWASIImports(t *testing.T) {
	// Module with no WASI imports should pass through unchanged (except _start rename).
	var buf []byte
	buf = append(buf, 0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00)

	// Type section: () -> ()
	var typeSec []byte
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 0)
	typeSec = appendTestULEB128(typeSec, 0)
	buf = appendTestSection(buf, 1, typeSec)

	// Import: just memory
	var importSec []byte
	importSec = appendTestULEB128(importSec, 1)
	importSec = appendTestString(importSec, "env")
	importSec = appendTestString(importSec, "memory")
	importSec = append(importSec, 0x02, 0x00)
	importSec = appendTestULEB128(importSec, 1)
	buf = appendTestSection(buf, 2, importSec)

	// Function section: 1 func
	var funcSec []byte
	funcSec = appendTestULEB128(funcSec, 1)
	funcSec = appendTestULEB128(funcSec, 0)
	buf = appendTestSection(buf, 3, funcSec)

	// Export: _start
	var exportSec []byte
	exportSec = appendTestULEB128(exportSec, 1)
	exportSec = appendTestString(exportSec, "_start")
	exportSec = append(exportSec, 0x00)
	exportSec = appendTestULEB128(exportSec, 0)
	buf = appendTestSection(buf, 7, exportSec)

	// Code section
	var codeSec []byte
	codeSec = appendTestULEB128(codeSec, 1)
	body := []byte{0x00, 0x0b} // 0 locals, end
	codeSec = appendTestULEB128(codeSec, uint32(len(body)))
	codeSec = append(codeSec, body...)
	buf = appendTestSection(buf, 10, codeSec)

	result, err := wasishim.RewriteWASIImportsForTest(buf)
	require.NoError(t, err)

	// Should rename _start to _initialize even with no WASI imports.
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "no_wasi.wasm")
	require.NoError(t, os.WriteFile(outPath, result, 0o644))

	wasmOpt, lookErr := exec.LookPath("wasm-opt")
	if lookErr != nil {
		t.Skip("wasm-opt not available")
	}

	cmd := exec.Command(wasmOpt, "--print", outPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "validation failed:\n%s", string(output))

	assert.Contains(t, string(output), "_initialize")
}

func TestRewriteWASIWithElementSection(t *testing.T) {
	// Build a module with WASI imports and an element section containing
	// function references that need remapping.
	var buf []byte
	buf = append(buf, 0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00)

	// Type section: (i32,i32)->i32 and ()->()
	var typeSec []byte
	typeSec = appendTestULEB128(typeSec, 2)
	// type 0: (i32,i32)->i32
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 2)
	typeSec = append(typeSec, 0x7f, 0x7f)
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x7f)
	// type 1: ()->()
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 0)
	typeSec = appendTestULEB128(typeSec, 0)
	buf = appendTestSection(buf, 1, typeSec)

	// Import: 1 WASI func import
	var importSec []byte
	importSec = appendTestULEB128(importSec, 1)
	importSec = appendTestString(importSec, "wasi_snapshot_preview1")
	importSec = appendTestString(importSec, "args_get")
	importSec = append(importSec, 0x00)
	importSec = appendTestULEB128(importSec, 0)
	buf = appendTestSection(buf, 2, importSec)

	// Function section: 2 local functions (func idx 1, 2)
	var funcSec []byte
	funcSec = appendTestULEB128(funcSec, 2)
	funcSec = appendTestULEB128(funcSec, 1) // func 1: type 1
	funcSec = appendTestULEB128(funcSec, 1) // func 2: type 1
	buf = appendTestSection(buf, 3, funcSec)

	// Table section: 1 funcref table, min=3
	var tableSec []byte
	tableSec = appendTestULEB128(tableSec, 1)
	tableSec = append(tableSec, 0x70) // funcref
	tableSec = append(tableSec, 0x00) // no max
	tableSec = appendTestULEB128(tableSec, 3)
	buf = appendTestSection(buf, 4, tableSec)

	// Export: func 1 as "_start"
	var exportSec []byte
	exportSec = appendTestULEB128(exportSec, 1)
	exportSec = appendTestString(exportSec, "_start")
	exportSec = append(exportSec, 0x00)
	exportSec = appendTestULEB128(exportSec, 1)
	buf = appendTestSection(buf, 7, exportSec)

	// Element section: 1 active segment, offset=0, refs=[func 1, func 2]
	var elemSec []byte
	elemSec = appendTestULEB128(elemSec, 1) // 1 segment
	elemSec = appendTestULEB128(elemSec, 0) // flags=0 (active)
	// offset expr: i32.const 0, end
	elemSec = append(elemSec, 0x41, 0x00, 0x0b)
	elemSec = appendTestULEB128(elemSec, 2) // 2 func refs
	elemSec = appendTestULEB128(elemSec, 1) // func 1
	elemSec = appendTestULEB128(elemSec, 2) // func 2
	buf = appendTestSection(buf, 9, elemSec)

	// Code section
	var codeSec []byte
	codeSec = appendTestULEB128(codeSec, 2)
	body := []byte{0x00, 0x0b} // 0 locals, end
	codeSec = appendTestULEB128(codeSec, uint32(len(body)))
	codeSec = append(codeSec, body...)
	codeSec = appendTestULEB128(codeSec, uint32(len(body)))
	codeSec = append(codeSec, body...)
	buf = appendTestSection(buf, 10, codeSec)

	result, err := wasishim.RewriteWASIImportsForTest(buf)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "elem_test.wasm")
	require.NoError(t, os.WriteFile(outPath, result, 0o644))

	wasmOpt, lookErr := exec.LookPath("wasm-opt")
	if lookErr != nil {
		t.Skip("wasm-opt not available")
	}

	cmd := exec.Command(wasmOpt, "--print", outPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "validation failed:\n%s", string(output))

	watOutput := string(output)
	assert.NotContains(t, watOutput, "wasi_snapshot_preview1")
	assert.Contains(t, watOutput, "_initialize")
}

func TestRewriteWASIWithAllKnownImports(t *testing.T) {
	// Test with all 15 known WASI imports to ensure all stubs are generated.
	wasiImports := []struct {
		name    string
		typeIdx uint32
	}{
		{"args_get", 0},
		{"args_sizes_get", 0},
		{"clock_time_get", 1},
		{"environ_get", 0},
		{"environ_sizes_get", 0},
		{"fd_write", 2},
		{"random_get", 0},
		{"poll_oneoff", 2},
		{"proc_exit", 3},
		{"sched_yield", 4},
		{"fd_close", 5},
		{"fd_fdstat_get", 0},
		{"fd_fdstat_set_flags", 0},
		{"fd_prestat_get", 0},
		{"fd_prestat_dir_name", 6},
	}

	var buf []byte
	buf = append(buf, 0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00)

	// Type section: 7 types matching wasm.go's buildShimWASM
	var typeSec []byte
	typeSec = appendTestULEB128(typeSec, 7)
	// type 0: (i32, i32) -> i32
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 2)
	typeSec = append(typeSec, 0x7f, 0x7f)
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x7f)
	// type 1: (i32, i64, i32) -> i32
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 3)
	typeSec = append(typeSec, 0x7f, 0x7e, 0x7f)
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x7f)
	// type 2: (i32, i32, i32, i32) -> i32
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 4)
	typeSec = append(typeSec, 0x7f, 0x7f, 0x7f, 0x7f)
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x7f)
	// type 3: (i32) -> ()
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x7f)
	typeSec = appendTestULEB128(typeSec, 0)
	// type 4: () -> i32
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 0)
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x7f)
	// type 5: (i32) -> i32
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x7f)
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x7f)
	// type 6: (i32, i32, i32) -> i32
	typeSec = append(typeSec, 0x60)
	typeSec = appendTestULEB128(typeSec, 3)
	typeSec = append(typeSec, 0x7f, 0x7f, 0x7f)
	typeSec = appendTestULEB128(typeSec, 1)
	typeSec = append(typeSec, 0x7f)

	buf = appendTestSection(buf, 1, typeSec)

	// Import section: all 15 WASI imports + 1 memory
	var importSec []byte
	importSec = appendTestULEB128(importSec, uint32(len(wasiImports)+1))
	for _, wi := range wasiImports {
		importSec = appendTestString(importSec, "wasi_snapshot_preview1")
		importSec = appendTestString(importSec, wi.name)
		importSec = append(importSec, 0x00) // func
		importSec = appendTestULEB128(importSec, wi.typeIdx)
	}
	// memory import
	importSec = appendTestString(importSec, "env")
	importSec = appendTestString(importSec, "memory")
	importSec = append(importSec, 0x02, 0x00)
	importSec = appendTestULEB128(importSec, 1)
	buf = appendTestSection(buf, 2, importSec)

	// Function section: 1 local func
	var funcSec []byte
	funcSec = appendTestULEB128(funcSec, 1)
	funcSec = appendTestULEB128(funcSec, 4) // type 4: () -> i32
	buf = appendTestSection(buf, 3, funcSec)

	// Export: func 15 as "_start" (0-14 are imports, 15 is local func 0)
	var exportSec []byte
	exportSec = appendTestULEB128(exportSec, 1)
	exportSec = appendTestString(exportSec, "_start")
	exportSec = append(exportSec, 0x00)
	exportSec = appendTestULEB128(exportSec, 15)
	buf = appendTestSection(buf, 7, exportSec)

	// Code section
	var codeSec []byte
	codeSec = appendTestULEB128(codeSec, 1)
	body := []byte{0x00, 0x41, 0x00, 0x0b} // 0 locals, i32.const 0, end
	codeSec = appendTestULEB128(codeSec, uint32(len(body)))
	codeSec = append(codeSec, body...)
	buf = appendTestSection(buf, 10, codeSec)

	result, err := wasishim.RewriteWASIImportsForTest(buf)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "all_wasi.wasm")
	require.NoError(t, os.WriteFile(outPath, result, 0o644))

	wasmOpt, lookErr := exec.LookPath("wasm-opt")
	if lookErr != nil {
		t.Skip("wasm-opt not available")
	}

	cmd := exec.Command(wasmOpt, "--print", outPath)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "wasm-opt validation failed:\n%s", string(output))

	watOutput := string(output)

	// Verify no WASI imports remain.
	assert.NotContains(t, watOutput, "wasi_snapshot_preview1")

	// Verify _start was renamed.
	assert.Contains(t, watOutput, "_initialize")

	// Verify the module has the expected number of functions.
	// wasm-opt may inline or split function printing, so just verify
	// there are at least 16 functions (1 local + 15 stubs).
	funcCount := strings.Count(watOutput, "(func ")
	assert.GreaterOrEqual(t, funcCount, 16, "expected at least 16 functions (1 local + 15 stubs)")

	t.Logf("All-WASI-imports module rewritten: %d bytes, %d functions in WAT", len(result), funcCount)
}
