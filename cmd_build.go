package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"go.digitalxero.dev/stdb-go/internal/wasishim"
)

func newBuildCmd() *cobra.Command {
	var (
		dir      string
		output   string
		optimize bool
		release  bool
		wasiShim bool
	)

	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a SpacetimeDB WASM module",
		Long: `Build compiles a Go SpacetimeDB module to a WASM binary.

This runs code generation (stdb-go) followed by a Go WASM build
targeting wasip1/wasm. Optionally runs wasm-opt for size optimization.`,
		Example: `  # Build the module in the current directory
  stdb-go build

  # Build with a specific output path
  stdb-go build --dir=./mymodule --output=mymodule.wasm

  # Build without optimization
  stdb-go build --optimize=false

  # Build without WASI import rewriting (for servers with host-side WASI stubs)
  stdb-go build --wasi-shim=false`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBuild(dir, output, optimize, release, wasiShim)
		},
	}

	cmd.Flags().StringVar(&dir, "dir", ".", "module directory containing Go source files")
	cmd.Flags().StringVar(&output, "output", "module.wasm", "output WASM file path")
	cmd.Flags().BoolVar(&optimize, "optimize", true, "run wasm-opt if available")
	cmd.Flags().BoolVar(&release, "release", true, "strip debug info with -ldflags=\"-s -w\"")
	cmd.Flags().BoolVar(&wasiShim, "wasi-shim", true, "rewrite WASI imports with local stubs")

	return cmd
}

func runBuild(dir, output string, optimize, release, wasiShim bool) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	// Step 1: Run code generation
	fmt.Fprintf(os.Stderr, "build: running code generation...\n")
	if err := runGenerateServer(absDir, "stdb_generated.go"); err != nil {
		return fmt.Errorf("build: codegen failed: %w", err)
	}

	// Step 2: Build WASM
	fmt.Fprintf(os.Stderr, "build: compiling WASM module...\n")

	absOutput := output
	if !filepath.IsAbs(output) {
		absOutput = filepath.Join(absDir, output)
	}

	goModCmd := exec.Command("go", "mod", "tidy")
	goModCmd.Dir = absDir
	goModCmd.Stdout = os.Stdout
	goModCmd.Stderr = os.Stderr

	if err = goModCmd.Run(); err != nil {
		return fmt.Errorf("build: go mod tidy failed: %w", err)
	}

	buildArgs := []string{"build", "-buildmode=c-shared", "-trimpath", "-tags=netgo,osusergo"}
	if release {
		buildArgs = append(buildArgs, `-ldflags=-s -w -extldflags "-static"`)
	}
	buildArgs = append(buildArgs, "-o", absOutput, ".")

	goCmd := exec.Command("go", buildArgs...)
	goCmd.Dir = absDir
	goCmd.Env = append(os.Environ(), "GOOS=wasip1", "GOARCH=wasm")
	goCmd.Stdout = os.Stdout
	goCmd.Stderr = os.Stderr

	if err = goCmd.Run(); err != nil {
		return fmt.Errorf("build: go build failed: %w", err)
	}

	// Step 3: Rewrite WASI Preview 1 imports with local stubs
	if wasiShim {
		fmt.Fprintf(os.Stderr, "build: rewriting WASI imports...\n")
		if err = wasishim.RewriteWASI(absOutput); err != nil {
			return fmt.Errorf("build: %w", err)
		}
	} else {
		fmt.Fprintf(os.Stderr, "build: skipping WASI import rewriting\n")
	}

	// Step 4: Optionally run wasm-opt
	if optimize {
		wasmOpt, lookErr := exec.LookPath("wasm-opt")
		if lookErr == nil {
			fmt.Fprintf(os.Stderr, "build: running wasm-opt...\n")
			optCmd := exec.Command(wasmOpt, "-all", "-g", "-O2", "-o", absOutput, absOutput)
			optCmd.Stdout = os.Stdout
			optCmd.Stderr = os.Stderr
			if err = optCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "build: wasm-opt failed (non-fatal): %v\n", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "build: wasm-opt not found, skipping optimization\n")
		}
	}

	info, err := os.Stat(absOutput)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	fmt.Fprintf(os.Stderr, "build: wrote %s (%d bytes)\n", absOutput, info.Size())
	return nil
}
