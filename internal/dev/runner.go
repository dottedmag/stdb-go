package dev

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// BuildFunc is called to build the WASM module.
type BuildFunc func(dir, output string, optimize, release, wasiShim bool) error

// PublishFunc is called to publish the WASM module to SpacetimeDB.
type PublishFunc func(dir, database, server, token, wasmFile string, clearDatabase, skipBuild, autoConfirm, wasiShim bool) error

// DevRunnerBuilder configures a DevRunner.
type DevRunnerBuilder interface {
	WithDir(dir string) DevRunnerBuilder
	WithDatabase(database string) DevRunnerBuilder
	WithServer(server string) DevRunnerBuilder
	WithToken(token string) DevRunnerBuilder
	WithDebounce(duration time.Duration) DevRunnerBuilder
	WithClearDatabase(clear bool) DevRunnerBuilder
	WithWasiShim(shim bool) DevRunnerBuilder
	WithClientCmd(cmd string) DevRunnerBuilder
	WithBuildFunc(fn BuildFunc) DevRunnerBuilder
	WithPublishFunc(fn PublishFunc) DevRunnerBuilder
	WithStderr(w io.Writer) DevRunnerBuilder
	Build() (DevRunner, error)
}

// DevRunner watches for file changes and rebuilds/publishes automatically.
type DevRunner interface {
	Run(ctx context.Context) error
}

type devRunner struct {
	dir           string
	database      string
	server        string
	token         string
	debounce      time.Duration
	clearDatabase bool
	wasiShim      bool
	clientCmd     string
	buildFunc     BuildFunc
	publishFunc   PublishFunc
	stderr        io.Writer
}

// NewDevRunner creates a new DevRunnerBuilder.
func NewDevRunner() DevRunnerBuilder {
	return &devRunner{
		debounce:      500 * time.Millisecond,
		clearDatabase: true,
		wasiShim:      true,
		stderr:        os.Stderr,
	}
}

func (r *devRunner) WithDir(dir string) DevRunnerBuilder {
	r.dir = dir
	return r
}

func (r *devRunner) WithDatabase(database string) DevRunnerBuilder {
	r.database = database
	return r
}

func (r *devRunner) WithServer(server string) DevRunnerBuilder {
	r.server = server
	return r
}

func (r *devRunner) WithToken(token string) DevRunnerBuilder {
	r.token = token
	return r
}

func (r *devRunner) WithDebounce(duration time.Duration) DevRunnerBuilder {
	r.debounce = duration
	return r
}

func (r *devRunner) WithClearDatabase(clear bool) DevRunnerBuilder {
	r.clearDatabase = clear
	return r
}

func (r *devRunner) WithWasiShim(shim bool) DevRunnerBuilder {
	r.wasiShim = shim
	return r
}

func (r *devRunner) WithClientCmd(cmd string) DevRunnerBuilder {
	r.clientCmd = cmd
	return r
}

func (r *devRunner) WithBuildFunc(fn BuildFunc) DevRunnerBuilder {
	r.buildFunc = fn
	return r
}

func (r *devRunner) WithPublishFunc(fn PublishFunc) DevRunnerBuilder {
	r.publishFunc = fn
	return r
}

func (r *devRunner) WithStderr(w io.Writer) DevRunnerBuilder {
	r.stderr = w
	return r
}

func (r *devRunner) Build() (DevRunner, error) {
	if r.buildFunc == nil {
		return nil, fmt.Errorf("dev: build function is required")
	}
	if r.publishFunc == nil {
		return nil, fmt.Errorf("dev: publish function is required")
	}
	if r.database == "" {
		return nil, fmt.Errorf("dev: database name is required")
	}
	if r.server == "" {
		r.server = "http://localhost:3000"
	}
	if r.dir == "" {
		r.dir = "."
	}
	return r, nil
}

func (r *devRunner) Run(ctx context.Context) error {
	// Step 1: Initial build+publish.
	_, _ = fmt.Fprintf(r.stderr, "dev: initial build and publish...\n")
	if err := r.buildAndPublish(true); err != nil {
		return fmt.Errorf("dev: initial build+publish failed: %w", err)
	}
	_, _ = fmt.Fprintf(r.stderr, "dev: initial publish complete\n")

	// Step 2: Start file watcher.
	watcher, err := NewFileWatcher().
		WithDir(r.dir).
		WithDebounce(r.debounce).
		Build()
	if err != nil {
		return fmt.Errorf("dev: creating file watcher: %w", err)
	}
	defer func() { _ = watcher.Close() }()

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	go watcher.Run(watchCtx)

	// Step 3: Start client process if configured.
	var clientProc *exec.Cmd
	if r.clientCmd != "" {
		clientProc, err = r.startClient(ctx)
		if err != nil {
			_, _ = fmt.Fprintf(r.stderr, "dev: warning: failed to start client: %v\n", err)
		} else {
			_, _ = fmt.Fprintf(r.stderr, "dev: started client: %s\n", r.clientCmd)
			defer r.stopClient(clientProc)
		}
	}

	// Step 4: Event loop.
	_, _ = fmt.Fprintf(r.stderr, "dev: watching for changes in %s...\n", r.dir)
	for {
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintf(r.stderr, "dev: shutting down...\n")
			return nil

		case _, ok := <-watcher.Events():
			if !ok {
				return nil
			}

			_, _ = fmt.Fprintf(r.stderr, "dev: change detected, rebuilding...\n")

			if err := r.buildAndPublish(false); err != nil {
				_, _ = fmt.Fprintf(r.stderr, "dev: build+publish failed: %v\n", err)
			} else {
				_, _ = fmt.Fprintf(r.stderr, "dev: rebuild complete\n")
			}

			// Drain any pending events that arrived during the build.
			draining := true
			for draining {
				select {
				case _, ok := <-watcher.Events():
					if !ok {
						return nil
					}
					_, _ = fmt.Fprintf(r.stderr, "dev: pending changes detected, rebuilding again...\n")
					if err := r.buildAndPublish(false); err != nil {
						_, _ = fmt.Fprintf(r.stderr, "dev: build+publish failed: %v\n", err)
					} else {
						_, _ = fmt.Fprintf(r.stderr, "dev: rebuild complete\n")
					}
				default:
					draining = false
				}
			}
		}
	}
}

func (r *devRunner) buildAndPublish(initial bool) error {
	clearDB := r.clearDatabase && initial
	return r.publishFunc(r.dir, r.database, r.server, r.token, "", clearDB, false, true, r.wasiShim)
}

func (r *devRunner) startClient(ctx context.Context) (*exec.Cmd, error) {
	parts := strings.Fields(r.clientCmd)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty client command")
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return cmd, nil
}

func (r *devRunner) stopClient(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	_ = cmd.Process.Signal(os.Interrupt)

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}
