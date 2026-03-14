//go:build integration

package main_test

import (
	"context"
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

func buildWASMModule(t *testing.T) []byte {
	t.Helper()

	moduleDir, err := filepath.Abs(testdataRoot())
	require.NoError(t, err)

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
			WithClearDatabase(true).
			Build()
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := pub.Publish(ctx, wasmBytes)
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
