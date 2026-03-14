package scaffold

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	goProxyURL = "https://proxy.golang.org"

	clientSDKModule          = "go.digitalxero.dev/spacetimedb-client"
	serverSDKModule          = "go.digitalxero.dev/spacetimedb-server"
	fallbackClientSDKVersion = "v0.5.0"
	fallbackServerSDKVersion = "v0.4.1"
)

type proxyResponse struct {
	Version string `json:"Version"`
}

// latestModuleVersion queries the Go module proxy for the latest version of a module.
// Returns the fallback version if the proxy is unreachable.
func latestModuleVersion(module, fallback string) string {
	client := &http.Client{Timeout: 5 * time.Second}

	resp, err := client.Get(fmt.Sprintf("%s/%s/@latest", goProxyURL, module))
	if err != nil {
		return fallback
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fallback
	}

	var pr proxyResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return fallback
	}

	if pr.Version == "" {
		return fallback
	}

	return pr.Version
}
