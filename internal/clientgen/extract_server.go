package clientgen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type serverExtractor struct {
	serverURL string
	database  string
	token     string
}

func (e *serverExtractor) Extract(ctx context.Context) (*ModuleSchema, error) {
	// Build the schema endpoint URL
	baseURL := strings.TrimRight(e.serverURL, "/")
	url := fmt.Sprintf("%s/v1/database/%s/schema", baseURL, e.database)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if e.token != "" {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching schema from %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Parse the JSON response into raw module def
	var rawDef RawModuleDef
	if err := json.Unmarshal(body, &rawDef); err != nil {
		return nil, fmt.Errorf("parsing schema JSON: %w", err)
	}

	// Resolve into ModuleSchema
	return resolveSchema(&rawDef)
}
