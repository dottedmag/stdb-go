//go:build integration

package clientgen_test

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

	"go.digitalxero.dev/stdb-go/internal/publish"
)

var (
	integrationServerURL string
	integrationDBName    string
)

func TestMain(m *testing.M) {
	integrationServerURL = envOrDefault("SPACETIMEDB_URL", "http://localhost:3000")
	integrationDBName = fmt.Sprintf("clientgen-integration-%d", time.Now().UnixNano())

	if err := waitForSpacetimeDB(integrationServerURL, 60*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "SpacetimeDB not ready: %v\n", err)
		os.Exit(1)
	}

	wasmBytes, err := buildAndGetWASMModule()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build WASM module: %v\n", err)
		os.Exit(1)
	}

	if err := publishModule(integrationServerURL, integrationDBName, wasmBytes); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to publish module: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Published module to %s database %q\n", integrationServerURL, integrationDBName)
	os.Exit(m.Run())
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func waitForSpacetimeDB(serverURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(serverURL + "/v1/ping")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Printf("SpacetimeDB is ready at %s\n", serverURL)
				return nil
			}
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("SpacetimeDB at %s did not become ready within %s", serverURL, timeout)
}

func buildAndGetWASMModule() ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "clientgen-integration-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Build stdb-go binary from source.
	binary := filepath.Join(tmpDir, "stdb-go")
	// The test working dir is internal/clientgen, so project root is ../..
	projectRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return nil, fmt.Errorf("resolving project root: %w", err)
	}

	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("building stdb-go binary: %w", err)
	}

	// Build WASM module.
	moduleDir, err := filepath.Abs(filepath.Join("..", "..", "testdata", "integration", "module"))
	if err != nil {
		return nil, fmt.Errorf("resolving module dir: %w", err)
	}

	wasiShim := envOrDefault("WASI_SHIM", "true")
	wasmOutput := filepath.Join(tmpDir, "module.wasm")
	cmd = exec.Command(binary, "build",
		"--dir="+moduleDir,
		"--output="+wasmOutput,
		"--optimize=false",
		"--wasi-shim="+wasiShim,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("stdb-go build failed: %w", err)
	}

	wasmBytes, err := os.ReadFile(wasmOutput)
	if err != nil {
		return nil, fmt.Errorf("reading WASM output: %w", err)
	}
	if len(wasmBytes) == 0 {
		return nil, fmt.Errorf("WASM module is empty")
	}

	fmt.Printf("Built WASM module: %d bytes\n", len(wasmBytes))
	return wasmBytes, nil
}

func publishModule(serverURL, dbName string, wasmBytes []byte) error {
	pub, err := publish.NewPublisherBuilder().
		WithServer(serverURL).
		WithDatabase(dbName).
		WithClearDatabase(true).
		Build()
	if err != nil {
		return fmt.Errorf("building publisher: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := pub.Publish(ctx, wasmBytes)
	if err != nil {
		return fmt.Errorf("publish failed: %w", err)
	}
	if result.PermissionDenied != nil {
		return fmt.Errorf("publish permission denied")
	}
	if result.Success == nil {
		return fmt.Errorf("publish success is nil")
	}
	if !strings.EqualFold("Created", result.Success.Op) {
		return fmt.Errorf("expected op 'Created', got %q", result.Success.Op)
	}

	return nil
}
