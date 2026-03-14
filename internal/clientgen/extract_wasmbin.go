package clientgen

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

type wasmExtractor struct {
	binPath string
}

func (e *wasmExtractor) Extract(ctx context.Context) (*ModuleSchema, error) {
	// Verify the WASM file exists
	if _, err := os.Stat(e.binPath); err != nil {
		return nil, fmt.Errorf("WASM binary not found: %w", err)
	}

	// Find spacetimedb-standalone or spacetime CLI
	cliPath, err := findSpacetimeCLI()
	if err != nil {
		return nil, err
	}

	// Run extract-schema command
	cmd := exec.CommandContext(ctx, cliPath, "extract-schema", e.binPath)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("extract-schema failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("extract-schema failed: %w", err)
	}

	// Parse the JSON output into raw module def
	var rawDef RawModuleDef
	if err := json.Unmarshal(output, &rawDef); err != nil {
		return nil, fmt.Errorf("parsing schema JSON: %w", err)
	}

	// Resolve into ModuleSchema
	return resolveSchema(&rawDef)
}

func findSpacetimeCLI() (string, error) {
	// Try spacetimedb-standalone first (preferred for extract-schema)
	if path, err := exec.LookPath("spacetimedb-standalone"); err == nil {
		return path, nil
	}

	// Fall back to spacetime CLI
	if path, err := exec.LookPath("spacetime"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("neither 'spacetimedb-standalone' nor 'spacetime' found on PATH; " +
		"install SpacetimeDB CLI or use --server/--database to fetch schema from a running server")
}
