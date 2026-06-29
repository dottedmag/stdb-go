package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type createIdentityResponse struct {
	Identity string `json:"identity"`
	Token    string `json:"token"`
}

// CreateIdentity mints a fresh identity and auth token from the server via
// POST {server}/v1/identity (empty body, no auth header). The returned token's
// identity becomes the owner of anything subsequently published with it. This
// mirrors the official CLI's local "direct login".
func CreateIdentity(ctx context.Context, server string) (identity, token string, err error) {
	u := strings.TrimRight(server, "/") + "/v1/identity"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return "", "", fmt.Errorf("creating identity request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("identity request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("reading identity response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("identity request returned status %d: %s", resp.StatusCode, string(body))
	}

	var out createIdentityResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", "", fmt.Errorf("parsing identity response: %w", err)
	}
	if out.Token == "" {
		return "", "", fmt.Errorf("identity response missing token")
	}
	return out.Identity, out.Token, nil
}
