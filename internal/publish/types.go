package publish

// PublishResult represents the JSON response from the publish API.
type PublishResult struct {
	Success          *PublishSuccess   `json:"Success,omitempty"`
	PermissionDenied *PermissionDenied `json:"PermissionDenied,omitempty"`
}

// PublishSuccess contains details of a successful publish operation.
type PublishSuccess struct {
	Domain           *string `json:"domain"`
	DatabaseIdentity string  `json:"database_identity"`
	Op               string  `json:"op"` // "Created" or "Updated"
}

// PermissionDenied is returned when the user lacks permission to publish.
type PermissionDenied struct {
	Name string `json:"name"`
}

// PrePublishResult represents the JSON response from the pre-publish check.
type PrePublishResult struct {
	AutoMigrate   *AutoMigrateResult   `json:"AutoMigrate,omitempty"`
	ManualMigrate *ManualMigrateResult `json:"ManualMigrate,omitempty"`
}

// AutoMigrateResult indicates the module can be auto-migrated.
type AutoMigrateResult struct {
	MigrationPlan string `json:"migration_plan"`
}

// ManualMigrateResult indicates the module requires manual migration.
type ManualMigrateResult struct {
	Summary   string   `json:"summary"`
	Details   []string `json:"details"`
	HasErrors bool     `json:"has_errors"`
}

// SpacetimeConfig represents the spacetime.json project config file.
type SpacetimeConfig struct {
	Database   string `json:"database"`
	Server     string `json:"server"`
	ModulePath string `json:"module-path"`
}

// CLIConfig represents the ~/.config/spacetime/cli.toml config file.
type CLIConfig struct {
	SpacetimeDBToken string         `toml:"spacetimedb_token"`
	DefaultServer    string         `toml:"default_server"`
	ServerConfigs    []ServerConfig `toml:"server_configs"`
}

// ServerConfig represents a named server entry in cli.toml.
type ServerConfig struct {
	Nickname string `toml:"nickname"`
	Host     string `toml:"host"`
	Protocol string `toml:"protocol"`
}

// ServerURL returns the full URL for this server config.
func (s ServerConfig) ServerURL() string {
	protocol := s.Protocol
	if protocol == "" {
		protocol = "http"
	}
	return protocol + "://" + s.Host
}
