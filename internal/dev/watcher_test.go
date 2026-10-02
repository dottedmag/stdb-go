package dev_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dottedmag/stdb-go/internal/dev"
)

func TestFileWatcher_Debounce(t *testing.T) {
	dir := t.TempDir()

	w, err := dev.NewFileWatcher().
		WithDir(dir).
		WithDebounce(100 * time.Millisecond).
		Build()
	require.NoError(t, err)
	defer func() { _ = w.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Write multiple files rapidly — should get a single event.
	for i := range 5 {
		f := filepath.Join(dir, "file"+string(rune('a'+i))+".go")
		require.NoError(t, os.WriteFile(f, []byte("package foo"), 0644))
		time.Sleep(10 * time.Millisecond)
	}

	// Should receive exactly one debounced event.
	select {
	case <-w.Events():
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for debounced event")
	}

	// Should NOT receive another event immediately.
	select {
	case <-w.Events():
		t.Fatal("received unexpected second event")
	case <-time.After(300 * time.Millisecond):
		// good — no duplicate
	}
}

func TestFileWatcher_IgnoresGeneratedFiles(t *testing.T) {
	dir := t.TempDir()

	w, err := dev.NewFileWatcher().
		WithDir(dir).
		WithDebounce(50 * time.Millisecond).
		Build()
	require.NoError(t, err)
	defer func() { _ = w.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Write files that should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stdb_generated.go"), []byte("package foo"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "foo_test.go"), []byte("package foo_test"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# readme"), 0644))

	// Should NOT receive any events.
	select {
	case <-w.Events():
		t.Fatal("received event for ignored file")
	case <-time.After(200 * time.Millisecond):
		// good
	}
}

func TestFileWatcher_DetectsGoFiles(t *testing.T) {
	dir := t.TempDir()

	w, err := dev.NewFileWatcher().
		WithDir(dir).
		WithDebounce(50 * time.Millisecond).
		Build()
	require.NoError(t, err)
	defer func() { _ = w.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Write a regular .go file — should trigger.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644))

	select {
	case <-w.Events():
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestFileWatcher_RecursiveSubdirectory(t *testing.T) {
	dir := t.TempDir()

	w, err := dev.NewFileWatcher().
		WithDir(dir).
		WithDebounce(50 * time.Millisecond).
		Build()
	require.NoError(t, err)
	defer func() { _ = w.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	// Create a subdirectory — this triggers an event for the dir creation.
	subDir := filepath.Join(dir, "pkg")
	require.NoError(t, os.Mkdir(subDir, 0755))

	// Drain the dir-creation event.
	select {
	case <-w.Events():
	case <-time.After(2 * time.Second):
		// might not fire if debounce absorbs it, that's ok
	}

	// Small delay for watcher to register the new subdir.
	time.Sleep(100 * time.Millisecond)

	// Write a .go file in the subdirectory — should trigger.
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "pkg.go"), []byte("package pkg"), 0644))

	select {
	case <-w.Events():
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event from subdirectory")
	}
}

func TestFileWatcher_DefaultDir(t *testing.T) {
	// Build with default dir should succeed (uses ".").
	w, err := dev.NewFileWatcher().
		WithDebounce(50 * time.Millisecond).
		Build()
	require.NoError(t, err)
	assert.NoError(t, w.Close())
}
