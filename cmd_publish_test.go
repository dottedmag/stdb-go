package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/publish"
)

func TestResolveOrCreateToken_FlagWins(t *testing.T) {
	tok, err := resolveOrCreateToken(context.Background(), "http://unused", "flagtok", nil)
	require.NoError(t, err)
	assert.Equal(t, "flagtok", tok)
}

func TestResolveOrCreateToken_UsesConfigToken(t *testing.T) {
	cfg := &publish.CLIConfig{SpacetimeDBToken: "cfgtok"}
	tok, err := resolveOrCreateToken(context.Background(), "http://unused", "", cfg)
	require.NoError(t, err)
	assert.Equal(t, "cfgtok", tok)
}

func TestResolveOrCreateToken_MintsAndSaves(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("LocalAppData", dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/identity", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"identity":"id-1","token":"minted-token"}`))
	}))
	defer srv.Close()

	tok, err := resolveOrCreateToken(context.Background(), srv.URL, "", nil)
	require.NoError(t, err)
	assert.Equal(t, "minted-token", tok)

	// The minted token is persisted and reloadable.
	cfg, err := publish.LoadCLIConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "minted-token", cfg.SpacetimeDBToken)
}

type fakePublisher struct {
	pre    *publish.PrePublishResult
	preErr error
}

func (f *fakePublisher) PrePublish(context.Context, []byte) (*publish.PrePublishResult, error) {
	return f.pre, f.preErr
}

func (f *fakePublisher) Publish(context.Context, []byte, publish.PublishOptions) (*publish.PublishResult, error) {
	return &publish.PublishResult{}, nil
}

func autoMigrate(breakClients, majorUpgrade bool) *publish.PrePublishResult {
	return &publish.PrePublishResult{AutoMigrate: &publish.AutoMigrateResult{
		MigratePlan:         "PLAN",
		BreakClients:        breakClients,
		Token:               json.RawMessage(`"tok123"`),
		MajorVersionUpgrade: majorUpgrade,
	}}
}

func manualMigrate(majorUpgrade bool) *publish.PrePublishResult {
	return &publish.PrePublishResult{ManualMigrate: &publish.ManualMigrateResult{
		Reason:              "column type changed",
		MajorVersionUpgrade: majorUpgrade,
	}}
}

func TestResolveMigration(t *testing.T) {
	cases := []struct {
		name         string
		pre          *publish.PrePublishResult
		clearMode    string
		breakClients bool
		autoConfirm  bool
		wantErr      string // substring; "" means no error
		wantPolicy   string
		wantToken    string
		wantClear    bool
	}{
		{
			name:      "new database",
			pre:       nil,
			clearMode: clearModeNever,
		},
		{
			name:      "compatible auto-migration",
			pre:       autoMigrate(false, false),
			clearMode: clearModeNever,
		},
		{
			name:      "break clients without flag aborts",
			pre:       autoMigrate(true, false),
			clearMode: clearModeNever,
			wantErr:   "BREAK existing clients",
		},
		{
			name:         "break clients with --break-clients",
			pre:          autoMigrate(true, false),
			clearMode:    clearModeNever,
			breakClients: true,
			wantPolicy:   "BreakClients",
			wantToken:    "tok123",
		},
		{
			name:        "break clients with --yes",
			pre:         autoMigrate(true, false),
			clearMode:   clearModeNever,
			autoConfirm: true,
			wantPolicy:  "BreakClients",
			wantToken:   "tok123",
		},
		{
			name:      "major version upgrade without --yes aborts",
			pre:       autoMigrate(false, true),
			clearMode: clearModeNever,
			wantErr:   "major version upgrade",
		},
		{
			name:        "major version upgrade with --yes",
			pre:         autoMigrate(false, true),
			clearMode:   clearModeNever,
			autoConfirm: true,
		},
		{
			name:      "manual migration never aborts",
			pre:       manualMigrate(false),
			clearMode: clearModeNever,
			wantErr:   "requires manual migration",
		},
		{
			name:      "manual migration on-conflict clears",
			pre:       manualMigrate(false),
			clearMode: clearModeOnConflict,
			wantClear: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub := &fakePublisher{pre: tc.pre}
			var opts publish.PublishOptions
			err := resolveMigration(context.Background(), pub, []byte("wasm"),
				tc.clearMode, tc.breakClients, tc.autoConfirm, &opts)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantPolicy, opts.Policy)
			assert.Equal(t, tc.wantToken, opts.MigrationToken)
			assert.Equal(t, tc.wantClear, opts.Clear)
		})
	}
}
