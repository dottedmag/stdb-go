package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// PublisherBuilder configures a Publisher with connection settings. The
// per-publish migration decisions (clear/policy/token/...) are passed to
// [Publisher.Publish] via [PublishOptions] instead, because some of them (the
// migration token in particular) are only known after [Publisher.PrePublish].
type PublisherBuilder interface {
	WithServer(server string) PublisherBuilder
	WithDatabase(database string) PublisherBuilder
	WithToken(token string) PublisherBuilder
	Build() (Publisher, error)
}

// Publisher handles publishing WASM modules to SpacetimeDB.
type Publisher interface {
	PrePublish(ctx context.Context, wasmBytes []byte) (*PrePublishResult, error)
	Publish(ctx context.Context, wasmBytes []byte, opts PublishOptions) (*PublishResult, error)
}

// PublishOptions are the per-request migration/creation parameters for a publish.
// They map directly onto the server's publish query parameters.
type PublishOptions struct {
	// Clear sends clear=true: destroy all existing data before publishing.
	Clear bool
	// Policy is the migration policy: "" (omitted, server default Compatible) or
	// "BreakClients" to approve a client-breaking auto-migration.
	Policy string
	// MigrationToken is the token from PrePublish's AutoMigrate result; required
	// (and only meaningful) when Policy is "BreakClients".
	MigrationToken string
	// NumReplicas, when non-nil, sets num_replicas.
	NumReplicas *uint
	// Parent, when non-empty, sets parent (only honored when creating a database).
	Parent string
	// Organization, when non-empty, sets org (only honored when creating a database).
	Organization string
}

// query builds the publish request's query parameters from the options. The
// host_type is always Wasm; everything else is included only when set.
func (o PublishOptions) query() url.Values {
	q := url.Values{}
	q.Set("host_type", "Wasm")
	if o.Clear {
		q.Set("clear", "true")
	}
	if o.Policy != "" {
		q.Set("policy", o.Policy)
	}
	if o.MigrationToken != "" {
		q.Set("token", o.MigrationToken)
	}
	if o.NumReplicas != nil {
		q.Set("num_replicas", strconv.FormatUint(uint64(*o.NumReplicas), 10))
	}
	if o.Parent != "" {
		q.Set("parent", o.Parent)
	}
	if o.Organization != "" {
		q.Set("org", o.Organization)
	}
	return q
}

type publisher struct {
	server   string
	database string
	token    string
	client   *http.Client
}

// NewPublisherBuilder creates a new PublisherBuilder.
func NewPublisherBuilder() PublisherBuilder {
	return &publisher{
		client: &http.Client{},
	}
}

func (p *publisher) WithServer(server string) PublisherBuilder {
	p.server = strings.TrimRight(server, "/")
	return p
}

func (p *publisher) WithDatabase(database string) PublisherBuilder {
	p.database = database
	return p
}

func (p *publisher) WithToken(token string) PublisherBuilder {
	p.token = token
	return p
}

func (p *publisher) Build() (Publisher, error) {
	if p.server == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if p.database == "" {
		return nil, fmt.Errorf("database name is required")
	}
	return p, nil
}

func (p *publisher) PrePublish(ctx context.Context, wasmBytes []byte) (*PrePublishResult, error) {
	encodedDB := url.PathEscape(p.database)
	u := fmt.Sprintf("%s/v1/database/%s/pre_publish?host_type=Wasm&pretty_print_style=NoColor", p.server, encodedDB)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(wasmBytes))
	if err != nil {
		return nil, fmt.Errorf("creating pre-publish request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pre-publish request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading pre-publish response: %w", err)
	}

	// 404 means new database — no migration needed
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pre-publish returned status %d: %s", resp.StatusCode, string(body))
	}

	var result PrePublishResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing pre-publish response: %w", err)
	}

	return &result, nil
}

func (p *publisher) Publish(ctx context.Context, wasmBytes []byte, opts PublishOptions) (*PublishResult, error) {
	encodedDB := url.PathEscape(p.database)
	u := fmt.Sprintf("%s/v1/database/%s?%s", p.server, encodedDB, opts.query().Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(wasmBytes))
	if err != nil {
		return nil, fmt.Errorf("creating publish request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("publish request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading publish response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("publish returned status %d: %s", resp.StatusCode, string(body))
	}

	var result PublishResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parsing publish response: %w", err)
	}

	return &result, nil
}
