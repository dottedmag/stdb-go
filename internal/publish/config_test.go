package publish_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dottedmag/stdb-go/internal/publish"
)

// useTempConfigDir points config resolution at a temp dir on all platforms.
func useTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("LocalAppData", dir) // Windows
	return dir
}

func TestSaveSpacetimeToken_FreshAndReload(t *testing.T) {
	useTempConfigDir(t)

	path, err := publish.SaveSpacetimeToken("tok-123")
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(path, "cli.toml"), "path %q", path)

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	cfg, err := publish.LoadCLIConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "tok-123", cfg.SpacetimeDBToken)
	// Fresh file seeds CLI-compatible defaults.
	assert.Equal(t, "maincloud", cfg.DefaultServer)
	_, ok := publish.FindServerConfig(cfg, "local")
	assert.True(t, ok, "default 'local' server should be seeded")
}

func TestSaveSpacetimeToken_PreservesExistingKeys(t *testing.T) {
	dir := useTempConfigDir(t)
	cfgDir := filepath.Join(dir, "spacetime")
	require.NoError(t, os.MkdirAll(cfgDir, 0o700))
	existing := `default_server = "local"
web_session_token = "web123"
spacetimedb_token = "old"

[[server_configs]]
nickname = "local"
host = "127.0.0.1:3000"
protocol = "http"
`
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "cli.toml"), []byte(existing), 0o600))

	_, err := publish.SaveSpacetimeToken("new-token")
	require.NoError(t, err)

	cfg, err := publish.LoadCLIConfig()
	require.NoError(t, err)
	assert.Equal(t, "new-token", cfg.SpacetimeDBToken)
	assert.Equal(t, "local", cfg.DefaultServer)
	sc, ok := publish.FindServerConfig(cfg, "local")
	require.True(t, ok)
	assert.Equal(t, "127.0.0.1:3000", sc.Host)

	// Unmodeled keys (web_session_token) must survive the read-modify-write.
	raw, err := os.ReadFile(filepath.Join(cfgDir, "cli.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(raw), "web_session_token")
	assert.Contains(t, string(raw), "web123")
}

func TestCreateIdentity(t *testing.T) {
	var gotMethod, gotAuth, gotPath string
	var gotBodyLen int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBodyLen = len(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"identity":"abc123","token":"jwt-xyz"}`))
	}))
	defer srv.Close()

	id, tok, err := publish.CreateIdentity(context.Background(), srv.URL+"/")
	require.NoError(t, err)
	assert.Equal(t, "abc123", id)
	assert.Equal(t, "jwt-xyz", tok)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "/v1/identity", gotPath)
	assert.Empty(t, gotAuth, "identity creation must be unauthenticated")
	assert.Equal(t, 0, gotBodyLen, "identity creation sends an empty body")
}

func TestCreateIdentity_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	_, _, err := publish.CreateIdentity(context.Background(), srv.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
