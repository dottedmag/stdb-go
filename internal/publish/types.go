package publish

import (
	"encoding/json"
	"strings"
)

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
// It is an externally-tagged enum: exactly one of the fields is set.
type PrePublishResult struct {
	AutoMigrate   *AutoMigrateResult   `json:"AutoMigrate,omitempty"`
	ManualMigrate *ManualMigrateResult `json:"ManualMigrate,omitempty"`
}

// AutoMigrateResult indicates the module change can be auto-migrated in place.
// When BreakClients is true the migration alters the client-visible schema, so
// the server requires it to be approved with policy=BreakClients and the Token
// echoed back on the publish request.
type AutoMigrateResult struct {
	MigratePlan         string          `json:"migrate_plan"`
	BreakClients        bool            `json:"break_clients"`
	Token               json.RawMessage `json:"token"` // opaque hash; echoed verbatim as token=
	MajorVersionUpgrade bool            `json:"major_version_upgrade"`
}

// QueryToken renders the opaque migration token as the value to pass back in the
// publish request's `token=` query parameter. The server serializes the token
// through the SATS serde bridge (a JSON string or number); either form is
// reduced to its underlying text here.
func (a *AutoMigrateResult) QueryToken() string {
	if len(a.Token) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(a.Token, &s); err == nil {
		return s
	}
	return strings.TrimSpace(string(a.Token))
}

// ManualMigrateResult indicates the module change cannot be auto-migrated; the
// only way forward is to clear the database's data.
type ManualMigrateResult struct {
	Reason              string `json:"reason"`
	MajorVersionUpgrade bool   `json:"major_version_upgrade"`
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
