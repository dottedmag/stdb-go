//go:build integration

package main_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/publish"
)

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func waitForSpacetimeDB(t *testing.T, serverURL string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(serverURL + "/v1/ping")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				t.Logf("SpacetimeDB is ready at %s", serverURL)
				return
			}
		}
		time.Sleep(1 * time.Second)
	}

	t.Fatalf("SpacetimeDB at %s did not become ready within %s", serverURL, timeout)
}

func buildStdbGo(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "stdb-go")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = filepath.Join(testdataRoot(), "..", "..", "..")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "failed to build stdb-go binary")

	return binary
}

func testdataRoot() string {
	// integration_test.go is in the project root
	return filepath.Join(".", "testdata", "integration", "module")
}

// refreshModuleSDKs upgrades the integration module's SpacetimeDB client and
// server SDK dependencies to their latest published versions. The integration
// test deliberately tracks the newest SDKs rather than a pinned snapshot, so it
// catches ABI drift (e.g. newly added host functions) as soon as it ships.
func refreshModuleSDKs(t *testing.T, moduleDir string) {
	t.Helper()

	cmd := exec.Command("go", "get",
		"go.digitalxero.dev/spacetimedb-client@latest",
		"go.digitalxero.dev/spacetimedb-server@latest",
	)
	cmd.Dir = moduleDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "failed to update module SDKs to latest")
}

func buildWASMModule(t *testing.T) []byte {
	t.Helper()

	moduleDir, err := filepath.Abs(testdataRoot())
	require.NoError(t, err)

	// Always build against the latest published SDKs.
	refreshModuleSDKs(t, moduleDir)

	// Build stdb-go binary from source.
	stdbGoBinary := buildStdbGo(t)

	// Run stdb-go build on the integration module.
	wasiShim := envOrDefault("WASI_SHIM", "true")
	wasmOutput := filepath.Join(t.TempDir(), "module.wasm")
	cmd := exec.Command(stdbGoBinary, "build",
		"--dir="+moduleDir,
		"--output="+wasmOutput,
		"--optimize=false",
		"--wasi-shim="+wasiShim,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "stdb-go build failed")

	wasmBytes, err := os.ReadFile(wasmOutput)
	require.NoError(t, err)
	require.NotEmpty(t, wasmBytes, "WASM module is empty")

	t.Logf("Built WASM module: %d bytes", len(wasmBytes))
	return wasmBytes
}

func TestIntegration(t *testing.T) {
	serverURL := envOrDefault("SPACETIMEDB_URL", "http://localhost:3000")
	dbName := fmt.Sprintf("integration-test-%d", time.Now().UnixNano())

	// Wait for SpacetimeDB to be ready.
	waitForSpacetimeDB(t, serverURL, 60*time.Second)

	// Build the WASM module.
	wasmBytes := buildWASMModule(t)

	t.Run("Publish", func(t *testing.T) {
		pub, err := publish.NewPublisherBuilder().
			WithServer(serverURL).
			WithDatabase(dbName).
			Build()
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := pub.Publish(ctx, wasmBytes, publish.PublishOptions{Clear: true})
		require.NoError(t, err, "publish failed")
		require.NotNil(t, result, "publish returned nil result")
		require.Nil(t, result.PermissionDenied, "publish permission denied")
		require.NotNil(t, result.Success, "publish success is nil")

		assert.True(t, strings.EqualFold("Created", result.Success.Op), "expected op 'Created' (case-insensitive), got %q", result.Success.Op)
		assert.NotEmpty(t, result.Success.DatabaseIdentity)

		t.Logf("Published database %q (identity: %s, op: %s)",
			dbName, result.Success.DatabaseIdentity, result.Success.Op)
	})

	// TODO: ClientConnect and ReducerRoundTrip subtests require the
	// go.digitalxero.dev/spacetimedb-client SDK which is not yet published.
	// Once available, add:
	//   t.Run("ClientConnect", ...) - connect via WebSocket, subscribe to player table
	//   t.Run("ReducerRoundTrip", ...) - call reducers, verify data via subscription callbacks
}

// TestBuildDeterminism guards against regression of the +25-bytes-per-rebuild
// drift reported by the OpenRPG team. Root cause: `go build -o file` recognizes
// the embedded go:buildid in a prior run's output and skips the write, so
// RewriteWASI ends up re-shimming its own previous output. The wrapPreinitWithInit
// pass is not idempotent, so each rebuild grows the module by ~25 bytes,
// eventually producing a binary that traps on __preinit__10_register at publish
// time. The fix in cmd_build.go removes the output file before invoking go build.
//
// This test does NOT require a running SpacetimeDB.
func TestBuildDeterminism(t *testing.T) {
	moduleDir, err := filepath.Abs(testdataRoot())
	require.NoError(t, err)

	// Always build against the latest published SDKs.
	refreshModuleSDKs(t, moduleDir)

	stdbGoBinary := buildStdbGo(t)
	wasmOutput := filepath.Join(t.TempDir(), "module.wasm")

	hashes := make([]string, 0, 3)
	for i := 1; i <= 3; i++ {
		cmd := exec.Command(stdbGoBinary, "build",
			"--dir="+moduleDir,
			"--output="+wasmOutput,
			"--optimize=false",
			"--wasi-shim=true",
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		require.NoError(t, cmd.Run(), "build %d failed", i)

		wasmBytes, err := os.ReadFile(wasmOutput)
		require.NoError(t, err)
		sum := sha256.Sum256(wasmBytes)
		h := hex.EncodeToString(sum[:])
		t.Logf("build %d: %d bytes, sha256=%s", i, len(wasmBytes), h)
		hashes = append(hashes, h)
	}

	for i := 1; i < len(hashes); i++ {
		assert.Equal(t, hashes[0], hashes[i],
			"build %d hash differs from build 1 — non-deterministic output", i+1)
	}
}
