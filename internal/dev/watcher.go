package dev

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileWatcherBuilder configures a FileWatcher.
type FileWatcherBuilder interface {
	WithDir(dir string) FileWatcherBuilder
	WithDebounce(duration time.Duration) FileWatcherBuilder
	Build() (FileWatcher, error)
}

// FileWatcher watches a directory tree for Go file changes.
type FileWatcher interface {
	Events() <-chan struct{}
	Run(ctx context.Context)
	Close() error
}

type fileWatcher struct {
	dir      string
	debounce time.Duration
	watcher  *fsnotify.Watcher
	events   chan struct{}
}

// NewFileWatcher creates a new FileWatcherBuilder.
func NewFileWatcher() FileWatcherBuilder {
	return &fileWatcher{
		debounce: 500 * time.Millisecond,
	}
}

func (w *fileWatcher) WithDir(dir string) FileWatcherBuilder {
	w.dir = dir
	return w
}

func (w *fileWatcher) WithDebounce(duration time.Duration) FileWatcherBuilder {
	w.debounce = duration
	return w
}

func (w *fileWatcher) Build() (FileWatcher, error) {
	if w.dir == "" {
		w.dir = "."
	}

	absDir, err := filepath.Abs(w.dir)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(absDir); err != nil {
		return nil, fmt.Errorf("watch directory: %w", err)
	}
	w.dir = absDir

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w.watcher = fsw
	w.events = make(chan struct{}, 1)

	if err := w.addRecursive(w.dir); err != nil {
		_ = fsw.Close()
		return nil, err
	}

	return w, nil
}

func (w *fileWatcher) Events() <-chan struct{} {
	return w.events
}

func (w *fileWatcher) Run(ctx context.Context) {
	var timer *time.Timer
	var timerC <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			if !w.isRelevant(event) {
				continue
			}

			// If a new directory was created, watch it recursively.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = w.addRecursive(event.Name)
				}
			}

			// Reset debounce timer.
			if timer == nil {
				timer = time.NewTimer(w.debounce)
				timerC = timer.C
			} else {
				timer.Reset(w.debounce)
			}

		case <-timerC:
			timer = nil
			timerC = nil
			// Non-blocking send — if there's already an event pending, skip.
			select {
			case w.events <- struct{}{}:
			default:
			}

		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			// Errors are logged but don't stop the watcher.
		}
	}
}

func (w *fileWatcher) Close() error {
	return w.watcher.Close()
}

// isRelevant returns true if the event is for a Go source file we care about.
func (w *fileWatcher) isRelevant(event fsnotify.Event) bool {
	name := filepath.Base(event.Name)

	// Ignore hidden files/dirs.
	if strings.HasPrefix(name, ".") {
		return false
	}

	// For directory creation events, always relevant (we may need to add a watch).
	if event.Has(fsnotify.Create) {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			return true
		}
	}

	// Only care about .go files.
	if !strings.HasSuffix(name, ".go") {
		return false
	}

	// Ignore generated and test files.
	if name == "stdb_generated.go" {
		return false
	}
	if strings.HasSuffix(name, "_test.go") {
		return false
	}

	return true
}

// addRecursive adds dir and all subdirectories to the watcher,
// skipping hidden dirs, vendor/, and build/.
func (w *fileWatcher) addRecursive(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
		}

		if !d.IsDir() {
			return nil
		}

		name := d.Name()
		if strings.HasPrefix(name, ".") || name == "vendor" || name == "build" {
			return filepath.SkipDir
		}

		return w.watcher.Add(path)
	})
}
