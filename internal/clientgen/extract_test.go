package clientgen_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dottedmag/stdb-go/internal/clientgen"
)

// --- Builder Validation Tests ---

func TestSchemaExtractorBuilder_MissingServerURL(t *testing.T) {
	_, err := clientgen.NewSchemaExtractor().
		FromServer("", "mydb", "").
		Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server URL")
}

func TestSchemaExtractorBuilder_MissingDatabase(t *testing.T) {
	_, err := clientgen.NewSchemaExtractor().
		FromServer("http://localhost", "", "").
		Build()
	require.Error(t, err)
}

func TestSchemaExtractorBuilder_BothMissing(t *testing.T) {
	_, err := clientgen.NewSchemaExtractor().
		FromServer("", "", "").
		Build()
	require.Error(t, err)
}

func TestSchemaExtractorBuilder_Success(t *testing.T) {
	ext, err := clientgen.NewSchemaExtractor().
		FromServer("http://localhost:3000", "mydb", "").
		Build()
	require.NoError(t, err)
	assert.NotNil(t, ext)
}

func TestSchemaExtractorBuilder_DefaultSchemaVersion(t *testing.T) {
	// Build an extractor and verify the default schema version is used in URL
	// We do this by making a request and checking the URL path
	var requestedURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
		writeMinimalSchema(w)
	}))
	defer ts.Close()

	ext, err := clientgen.NewSchemaExtractor().
		FromServer(ts.URL, "testdb", "").
		Build()
	require.NoError(t, err)

	_, _ = ext.Extract(context.Background())
	assert.Contains(t, requestedURL, "version=10")
}

func TestSchemaExtractorBuilder_CustomSchemaVersion(t *testing.T) {
	var requestedURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedURL = r.URL.String()
		w.WriteHeader(http.StatusOK)
		writeMinimalSchema(w)
	}))
	defer ts.Close()

	ext, err := clientgen.NewSchemaExtractor().
		FromServer(ts.URL, "testdb", "").
		WithSchemaVersion("9").
		Build()
	require.NoError(t, err)

	_, _ = ext.Extract(context.Background())
	assert.Contains(t, requestedURL, "version=9")
}

// --- Server Extraction Tests (httptest) ---

func TestServerExtractor_HTTPError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"not_found", http.StatusNotFound},
		{"server_error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte("error response"))
			}))
			defer ts.Close()

			_, err := clientgen.ExtractFromServerForTest(context.Background(), ts.URL, "testdb", "")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "server returned")
		})
	}
}

func TestServerExtractor_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json{{{"))
	}))
	defer ts.Close()

	_, err := clientgen.ExtractFromServerForTest(context.Background(), ts.URL, "testdb", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing schema JSON")
}

func TestServerExtractor_AuthorizationHeader(t *testing.T) {
	var authHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		writeMinimalSchema(w)
	}))
	defer ts.Close()

	_, _ = clientgen.ExtractFromServerForTest(context.Background(), ts.URL, "testdb", "my-secret-token")
	assert.Equal(t, "Bearer my-secret-token", authHeader)
}

func TestServerExtractor_NoAuthWhenEmpty(t *testing.T) {
	var hasAuth bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasAuth = r.Header.Get("Authorization") != ""
		w.WriteHeader(http.StatusOK)
		writeMinimalSchema(w)
	}))
	defer ts.Close()

	_, _ = clientgen.ExtractFromServerForTest(context.Background(), ts.URL, "testdb", "")
	assert.False(t, hasAuth)
}

func TestServerExtractor_URLConstruction(t *testing.T) {
	var requestedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path + "?" + r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		writeMinimalSchema(w)
	}))
	defer ts.Close()

	_, _ = clientgen.ExtractFromServerForTest(context.Background(), ts.URL, "my-database", "")
	assert.Equal(t, "/v1/database/my-database/schema?version=10", requestedPath)
}

func TestServerExtractor_TrailingSlash(t *testing.T) {
	var requestedPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		writeMinimalSchema(w)
	}))
	defer ts.Close()

	// Use URL with trailing slash
	_, _ = clientgen.ExtractFromServerForTest(context.Background(), ts.URL+"/", "testdb", "")
	assert.Equal(t, "/v1/database/testdb/schema", requestedPath)
	assert.NotContains(t, requestedPath, "//")
}

func TestServerExtractor_ContextCancellation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeMinimalSchema(w)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := clientgen.ExtractFromServerForTest(ctx, ts.URL, "testdb", "")
	require.Error(t, err)
}

func TestServerExtractor_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		writeBasicSchema(w)
	}))
	defer ts.Close()

	schema, err := clientgen.ExtractFromServerForTest(context.Background(), ts.URL, "testdb", "")
	require.NoError(t, err)
	require.NotNil(t, schema)
	assert.Len(t, schema.Tables, 1)
	assert.Equal(t, "Player", schema.Tables[0].Name)
	assert.Len(t, schema.Reducers, 2)
}

// --- Helpers ---

func writeMinimalSchema(w http.ResponseWriter) {
	schema := map[string]any{
		"V10": map[string]any{
			"sections": []any{
				map[string]any{"Typespace": []any{}},
			},
		},
	}
	_ = json.NewEncoder(w).Encode(schema)
}

func writeBasicSchema(w http.ResponseWriter) {
	schema := map[string]any{
		"V10": map[string]any{
			"sections": []any{
				map[string]any{
					"Typespace": []any{
						map[string]any{
							"Product": map[string]any{
								"elements": []any{
									map[string]any{"name": "id", "algebraic_type": map[string]any{"U64": nil}},
									map[string]any{"name": "name", "algebraic_type": map[string]any{"String": nil}},
									map[string]any{"name": "score", "algebraic_type": map[string]any{"U32": nil}},
								},
							},
						},
					},
				},
				map[string]any{
					"Types": []any{
						map[string]any{
							"source_name":     map[string]any{"scope": []any{}, "source_name": "Player"},
							"ty":              0,
							"custom_ordering": false,
						},
					},
				},
				map[string]any{
					"Tables": []any{
						map[string]any{
							"source_name":      "Player",
							"product_type_ref": 0,
							"primary_key":      0,
							"indexes":          []any{},
							"constraints":      []any{},
							"sequences":        []any{},
							"table_type":       "User",
							"table_access":     "Public",
							"default_values":   []any{},
							"is_event":         false,
						},
					},
				},
				map[string]any{
					"Reducers": []any{
						map[string]any{
							"source_name":     "add_player",
							"params":          map[string]any{"elements": []any{map[string]any{"name": "name", "algebraic_type": map[string]any{"String": nil}}, map[string]any{"name": "score", "algebraic_type": map[string]any{"U32": nil}}}},
							"visibility":      "ClientCallable",
							"ok_return_type":  map[string]any{"Product": map[string]any{"elements": []any{}}},
							"err_return_type": map[string]any{"String": nil},
						},
						map[string]any{
							"source_name":     "update_score",
							"params":          map[string]any{"elements": []any{map[string]any{"name": "player_id", "algebraic_type": map[string]any{"U64": nil}}, map[string]any{"name": "new_score", "algebraic_type": map[string]any{"U32": nil}}}},
							"visibility":      "ClientCallable",
							"ok_return_type":  map[string]any{"Product": map[string]any{"elements": []any{}}},
							"err_return_type": map[string]any{"String": nil},
						},
					},
				},
			},
		},
	}
	_ = json.NewEncoder(w).Encode(schema)
}
