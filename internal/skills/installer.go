package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// InstallerBuilder configures and builds an Installer.
type InstallerBuilder interface {
	WithFS(fsys fs.FS) InstallerBuilder
	WithOutDir(dir string) InstallerBuilder
	Build() (Installer, error)
}

// Installer extracts bundled agent skills into an output directory.
type Installer interface {
	// Install syncs every skill file into the output directory, creating
	// directories as needed and overwriting existing files. It returns the
	// slash-separated relative paths that were written.
	Install() ([]string, error)
	// OutDir returns the resolved (tilde-expanded, cleaned) output directory.
	OutDir() string
}

type installer struct {
	fsys   fs.FS
	outDir string
}

// NewInstallerBuilder creates a new InstallerBuilder.
func NewInstallerBuilder() InstallerBuilder {
	return &installer{}
}

func (i *installer) WithFS(fsys fs.FS) InstallerBuilder {
	i.fsys = fsys
	return i
}

func (i *installer) WithOutDir(dir string) InstallerBuilder {
	i.outDir = dir
	return i
}

func (i *installer) Build() (Installer, error) {
	if i.fsys == nil {
		return nil, fmt.Errorf("skills: source filesystem is required")
	}
	if i.outDir == "" {
		return nil, fmt.Errorf("skills: output directory is required")
	}

	dir, err := expandHome(i.outDir)
	if err != nil {
		return nil, err
	}
	i.outDir = filepath.Clean(dir)

	return i, nil
}

func (i *installer) OutDir() string {
	return i.outDir
}

func (i *installer) Install() ([]string, error) {
	var written []string

	err := fs.WalkDir(i.fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("skills: walking embedded skills: %w", err)
		}
		// Skip .go files so the embed package's own sources are never
		// installed, regardless of how the source FS was assembled.
		if d.IsDir() || strings.HasSuffix(path, ".go") {
			return nil
		}

		data, err := fs.ReadFile(i.fsys, path)
		if err != nil {
			return fmt.Errorf("skills: reading %s: %w", path, err)
		}

		dest := filepath.Join(i.outDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return fmt.Errorf("skills: creating directory %s: %w", filepath.Dir(dest), err)
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return fmt.Errorf("skills: writing %s: %w", dest, err)
		}

		written = append(written, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return written, nil
}

// expandHome resolves a leading "~" or "~/" in path. The shell normally
// expands an unquoted ~ before the binary sees it; this covers quoted or
// scripted invocations. Other users' homes ("~name") are left untouched.
func expandHome(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("skills: resolving home directory: %w", err)
	}

	if path == "~" {
		return home, nil
	}

	return filepath.Join(home, path[2:]), nil
}
