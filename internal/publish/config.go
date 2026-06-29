package publish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

// spacetimeConfigDir resolves the directory holding the spacetime CLI config,
// matching the official CLI (which uses XDG even on macOS): $XDG_CONFIG_HOME/spacetime,
// else ~/.config/spacetime; on Windows %LocalAppData%\SpacetimeDB\config.
//
// Note: this intentionally does NOT use os.UserConfigDir(), which on macOS returns
// ~/Library/Application Support and would not find the CLI's cli.toml.
func spacetimeConfigDir() (string, error) {
	if runtime.GOOS == "windows" {
		if lad := os.Getenv("LocalAppData"); lad != "" {
			return filepath.Join(lad, "SpacetimeDB", "config"), nil
		}
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "spacetime"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".config", "spacetime"), nil
}

// cliConfigPath returns the full path to cli.toml.
func cliConfigPath() (string, error) {
	dir, err := spacetimeConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cli.toml"), nil
}

// LoadSpacetimeConfig reads spacetime.json from the given directory.
// Returns nil with no error if the file doesn't exist.
func LoadSpacetimeConfig(dir string) (*SpacetimeConfig, error) {
	path := filepath.Join(dir, "spacetime.json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading spacetime.json: %w", err)
	}

	var cfg SpacetimeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing spacetime.json: %w", err)
	}

	return &cfg, nil
}

// LoadCLIConfig reads the spacetime CLI config (cli.toml).
// Returns nil with no error if the file doesn't exist.
func LoadCLIConfig() (*CLIConfig, error) {
	path, err := cliConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading cli.toml: %w", err)
	}

	var cfg CLIConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing cli.toml: %w", err)
	}

	return &cfg, nil
}

// SaveSpacetimeToken persists the auth token as the top-level spacetimedb_token in
// cli.toml, creating the file (and config dir) if needed. Existing keys are
// preserved (read-modify-write). When creating a fresh file it seeds the same
// default servers the official CLI uses, so the result is CLI-compatible. The
// write is atomic (temp file + rename) and the file mode is 0600. Returns the path.
func SaveSpacetimeToken(token string) (string, error) {
	dir, err := spacetimeConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "cli.toml")

	cfg := map[string]any{}
	switch data, err := os.ReadFile(path); {
	case err == nil:
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return "", fmt.Errorf("parsing existing cli.toml: %w", err)
		}
	case os.IsNotExist(err):
		// Fresh file: seed CLI-compatible defaults.
		cfg["default_server"] = "maincloud"
		cfg["server_configs"] = []map[string]any{
			{"nickname": "maincloud", "host": "maincloud.spacetimedb.com", "protocol": "https"},
			{"nickname": "local", "host": "127.0.0.1:3000", "protocol": "http"},
		}
	default:
		return "", fmt.Errorf("reading cli.toml: %w", err)
	}

	cfg["spacetimedb_token"] = token

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return "", fmt.Errorf("encoding cli.toml: %w", err)
	}
	if err := atomicWriteFile(path, buf.Bytes(), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// atomicWriteFile writes data to path atomically: a temp file in the same
// directory is written then renamed over the target.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cli.toml.tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// FindServerConfig looks up a server by nickname in the CLI config.
func FindServerConfig(cfg *CLIConfig, nickname string) (*ServerConfig, bool) {
	if cfg == nil {
		return nil, false
	}

	for i := range cfg.ServerConfigs {
		if cfg.ServerConfigs[i].Nickname == nickname {
			return &cfg.ServerConfigs[i], true
		}
	}

	return nil, false
}

// ResolveToken determines the auth token from flag, env var, or cli.toml.
func ResolveToken(flagToken string, cliCfg *CLIConfig) string {
	if flagToken != "" {
		return flagToken
	}

	if envToken := os.Getenv("SPACETIMEDB_TOKEN"); envToken != "" {
		return envToken
	}

	if cliCfg != nil && cliCfg.SpacetimeDBToken != "" {
		return cliCfg.SpacetimeDBToken
	}

	return ""
}

// ResolveServer determines the server URL from flag, spacetime.json, or cli.toml defaults.
func ResolveServer(flagServer string, spacetimeCfg *SpacetimeConfig, cliCfg *CLIConfig) string {
	// Explicit flag takes priority
	if flagServer != "" {
		return flagServer
	}

	// Try spacetime.json server field → look up in cli.toml
	if spacetimeCfg != nil && spacetimeCfg.Server != "" {
		if sc, ok := FindServerConfig(cliCfg, spacetimeCfg.Server); ok {
			return sc.ServerURL()
		}
	}

	// Try cli.toml default server
	if cliCfg != nil && cliCfg.DefaultServer != "" {
		if sc, ok := FindServerConfig(cliCfg, cliCfg.DefaultServer); ok {
			return sc.ServerURL()
		}
	}

	return "http://localhost:3000"
}

// ResolveDatabase determines the database name from flag or spacetime.json.
func ResolveDatabase(flagDB string, spacetimeCfg *SpacetimeConfig) string {
	if flagDB != "" {
		return flagDB
	}

	if spacetimeCfg != nil && spacetimeCfg.Database != "" {
		return spacetimeCfg.Database
	}

	return ""
}
