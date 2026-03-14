package publish

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

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

// LoadCLIConfig reads ~/.config/spacetime/cli.toml.
// Returns nil with no error if the file doesn't exist.
func LoadCLIConfig() (*CLIConfig, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolving config dir: %w", err)
	}

	path := filepath.Join(configDir, "spacetime", "cli.toml")

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
