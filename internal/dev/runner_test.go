package dev_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/dev"
)

func TestDevRunner_BuildRequired(t *testing.T) {
	_, err := dev.NewDevRunner().
		WithDatabase("test").
		WithPublishFunc(func(string, string, string, string, string, bool, bool, bool, bool) error { return nil }).
		Build()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "build function is required")
}

func TestDevRunner_PublishRequired(t *testing.T) {
	_, err := dev.NewDevRunner().
		WithDatabase("test").
		WithBuildFunc(func(string, string, bool, bool, bool) error { return nil }).
		Build()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "publish function is required")
}

func TestDevRunner_DatabaseRequired(t *testing.T) {
	_, err := dev.NewDevRunner().
		WithBuildFunc(func(string, string, bool, bool, bool) error { return nil }).
		WithPublishFunc(func(string, string, string, string, string, bool, bool, bool, bool) error { return nil }).
		Build()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database name is required")
}

func TestDevRunner_InitialBuildAndPublish(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644))

	var publishCount atomic.Int32
	var stderr bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())

	runner, err := dev.NewDevRunner().
		WithDir(dir).
		WithDatabase("test-db").
		WithServer("http://localhost:3000").
		WithDebounce(50 * time.Millisecond).
		WithBuildFunc(func(string, string, bool, bool, bool) error { return nil }).
		WithPublishFunc(func(d, db, s, tok, wasm string, clear, skip, auto, shim bool) error {
			publishCount.Add(1)
			// After first publish, cancel so Run() exits.
			if publishCount.Load() == 1 {
				assert.True(t, clear, "initial publish should clear database")
				cancel()
			}
			return nil
		}).
		WithStderr(&stderr).
		Build()
	require.NoError(t, err)

	err = runner.Run(ctx)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, publishCount.Load(), int32(1))
}

func TestDevRunner_BuildErrorNonFatal(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644))

	var publishCount atomic.Int32
	var stderr bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())

	runner, err := dev.NewDevRunner().
		WithDir(dir).
		WithDatabase("test-db").
		WithDebounce(50 * time.Millisecond).
		WithBuildFunc(func(string, string, bool, bool, bool) error { return nil }).
		WithPublishFunc(func(d, db, s, tok, wasm string, clear, skip, auto, shim bool) error {
			count := publishCount.Add(1)
			if count == 1 {
				// Initial publish succeeds.
				return nil
			}
			if count == 2 {
				// Second publish fails — should not kill the loop.
				return fmt.Errorf("simulated build error")
			}
			// Third publish — cancel to exit.
			cancel()
			return nil
		}).
		WithStderr(&stderr).
		Build()
	require.NoError(t, err)

	// Trigger changes after initial build.
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, "handler.go"), []byte("package main\nfunc handler() {}"), 0644)
		time.Sleep(300 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, "model.go"), []byte("package main\nfunc model() {}"), 0644)
	}()

	err = runner.Run(ctx)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, publishCount.Load(), int32(2), "should have attempted at least 2 publishes")
	assert.Contains(t, stderr.String(), "build+publish failed")
}

func TestDevRunner_ClearDatabaseOnlyOnInitial(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644))

	var publishCount atomic.Int32
	clearDBValues := make([]bool, 0, 3)
	var stderr bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())

	runner, err := dev.NewDevRunner().
		WithDir(dir).
		WithDatabase("test-db").
		WithClearDatabase(true).
		WithDebounce(50 * time.Millisecond).
		WithBuildFunc(func(string, string, bool, bool, bool) error { return nil }).
		WithPublishFunc(func(d, db, s, tok, wasm string, clear, skip, auto, shim bool) error {
			count := publishCount.Add(1)
			clearDBValues = append(clearDBValues, clear)
			if count >= 2 {
				cancel()
			}
			return nil
		}).
		WithStderr(&stderr).
		Build()
	require.NoError(t, err)

	// Trigger a change after initial build.
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\nfunc new() {}"), 0644)
	}()

	err = runner.Run(ctx)
	assert.NoError(t, err)

	if len(clearDBValues) >= 2 {
		assert.True(t, clearDBValues[0], "initial publish should clear database")
		assert.False(t, clearDBValues[1], "subsequent publishes should NOT clear database")
	}
}

func TestDevRunner_InitialPublishFailure(t *testing.T) {
	dir := t.TempDir()

	var stderr bytes.Buffer

	runner, err := dev.NewDevRunner().
		WithDir(dir).
		WithDatabase("test-db").
		WithBuildFunc(func(string, string, bool, bool, bool) error { return nil }).
		WithPublishFunc(func(string, string, string, string, string, bool, bool, bool, bool) error {
			return fmt.Errorf("connection refused")
		}).
		WithStderr(&stderr).
		Build()
	require.NoError(t, err)

	err = runner.Run(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "initial build+publish failed")
}
