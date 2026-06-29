package publish_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.digitalxero.dev/stdb-go/internal/publish"
)

func uintp(v uint) *uint { return &v }

func TestPublish_QueryParams(t *testing.T) {
	cases := []struct {
		name   string
		opts   publish.PublishOptions
		want   map[string]string
		absent []string
	}{
		{
			name:   "plain compatible publish",
			opts:   publish.PublishOptions{},
			want:   map[string]string{"host_type": "Wasm"},
			absent: []string{"clear", "policy", "token", "num_replicas", "parent", "org"},
		},
		{
			name: "clear",
			opts: publish.PublishOptions{Clear: true},
			want: map[string]string{"host_type": "Wasm", "clear": "true"},
		},
		{
			name: "break clients sends policy + token",
			opts: publish.PublishOptions{Policy: "BreakClients", MigrationToken: "deadbeef"},
			want: map[string]string{"host_type": "Wasm", "policy": "BreakClients", "token": "deadbeef"},
		},
		{
			name:   "creation params",
			opts:   publish.PublishOptions{NumReplicas: uintp(3), Parent: "par", Organization: "acme"},
			want:   map[string]string{"host_type": "Wasm", "num_replicas": "3", "parent": "par", "org": "acme"},
			absent: []string{"clear", "policy", "token"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotQuery url.Values
			var gotMethod, gotAuth, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotQuery = r.URL.Query()
				gotMethod = r.Method
				gotAuth = r.Header.Get("Authorization")
				gotPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"Success":{"database_identity":"id","op":"Updated"}}`))
			}))
			defer srv.Close()

			pub, err := publish.NewPublisherBuilder().
				WithServer(srv.URL).WithDatabase("my-db").WithToken("tok").Build()
			require.NoError(t, err)

			res, err := pub.Publish(context.Background(), []byte("wasm"), tc.opts)
			require.NoError(t, err)
			require.NotNil(t, res.Success)

			assert.Equal(t, http.MethodPut, gotMethod)
			assert.Equal(t, "/v1/database/my-db", gotPath)
			assert.Equal(t, "Bearer tok", gotAuth)
			for k, v := range tc.want {
				assert.Equal(t, v, gotQuery.Get(k), "query param %q", k)
			}
			for _, k := range tc.absent {
				assert.Empty(t, gotQuery.Get(k), "query param %q should be absent", k)
			}
		})
	}
}

func TestPrePublish_ParsesAutoMigrate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/database/my-db/pre_publish", r.URL.Path)
		assert.Equal(t, "Wasm", r.URL.Query().Get("host_type"))
		assert.Equal(t, "NoColor", r.URL.Query().Get("pretty_print_style"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"AutoMigrate":{"migrate_plan":"PLAN","break_clients":true,"token":"deadbeef","major_version_upgrade":false}}`))
	}))
	defer srv.Close()

	pub, err := publish.NewPublisherBuilder().WithServer(srv.URL).WithDatabase("my-db").Build()
	require.NoError(t, err)

	res, err := pub.PrePublish(context.Background(), []byte("wasm"))
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.AutoMigrate)
	assert.Nil(t, res.ManualMigrate)
	assert.Equal(t, "PLAN", res.AutoMigrate.MigratePlan)
	assert.True(t, res.AutoMigrate.BreakClients)
	assert.False(t, res.AutoMigrate.MajorVersionUpgrade)
	assert.Equal(t, "deadbeef", res.AutoMigrate.QueryToken())
}

func TestPrePublish_ParsesManualMigrate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ManualMigrate":{"reason":"column type changed","major_version_upgrade":true}}`))
	}))
	defer srv.Close()

	pub, _ := publish.NewPublisherBuilder().WithServer(srv.URL).WithDatabase("my-db").Build()
	res, err := pub.PrePublish(context.Background(), []byte("wasm"))
	require.NoError(t, err)
	require.NotNil(t, res.ManualMigrate)
	assert.Nil(t, res.AutoMigrate)
	assert.Equal(t, "column type changed", res.ManualMigrate.Reason)
	assert.True(t, res.ManualMigrate.MajorVersionUpgrade)
}

func TestPrePublish_404IsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	pub, _ := publish.NewPublisherBuilder().WithServer(srv.URL).WithDatabase("my-db").Build()
	res, err := pub.PrePublish(context.Background(), []byte("wasm"))
	require.NoError(t, err)
	assert.Nil(t, res, "a new database (404) yields a nil result")
}

func TestQueryToken(t *testing.T) {
	assert.Equal(t, "abc", (&publish.AutoMigrateResult{Token: json.RawMessage(`"abc"`)}).QueryToken())
	assert.Equal(t, "12345", (&publish.AutoMigrateResult{Token: json.RawMessage(`12345`)}).QueryToken())
	assert.Equal(t, "", (&publish.AutoMigrateResult{}).QueryToken())
}
