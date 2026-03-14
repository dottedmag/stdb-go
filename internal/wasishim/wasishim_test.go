package wasishim_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/wasishim"
)

func TestBuildShimWASM(t *testing.T) {
	wasmBytes := wasishim.BuildShimWASMForTest()

	// Verify WASM magic number and version.
	require.True(t, len(wasmBytes) > 8, "WASM module too small")
	assert.Equal(t, []byte{0x00, 0x61, 0x73, 0x6d}, wasmBytes[:4], "invalid WASM magic")
	assert.Equal(t, []byte{0x01, 0x00, 0x00, 0x00}, wasmBytes[4:8], "invalid WASM version")

	t.Logf("Generated WASI shim WASM: %d bytes", len(wasmBytes))
}

func TestBuildShimWASMValidatesWithWasmOpt(t *testing.T) {
	wasmOpt, err := exec.LookPath("wasm-opt")
	if err != nil {
		t.Skip("wasm-opt not available, skipping validation")
	}

	wasmBytes := wasishim.BuildShimWASMForTest()

	tmpDir := t.TempDir()
	shimPath := filepath.Join(tmpDir, "shim.wasm")
	require.NoError(t, os.WriteFile(shimPath, wasmBytes, 0o644))

	// wasm-opt --print validates the module and prints WAT.
	cmd := exec.Command(wasmOpt, "--debug", "--print", shimPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("shim.wasm written to: %s", shimPath)
	}
	require.NoError(t, err, "wasm-opt validation failed:\n%s", string(output))

	t.Logf("wasm-opt validated shim module successfully")
}
