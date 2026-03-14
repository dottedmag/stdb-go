package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// PublisherBuilder configures a Publisher.
type PublisherBuilder interface {
	WithServer(server string) PublisherBuilder
	WithDatabase(database string) PublisherBuilder
	WithToken(token string) PublisherBuilder
	WithClearDatabase(clear bool) PublisherBuilder
	WithAutoConfirm(confirm bool) PublisherBuilder
	Build() (Publisher, error)
}

// Publisher handles publishing WASM modules to SpacetimeDB.
type Publisher interface {
	PrePublish(ctx context.Context, wasmBytes []byte) (*PrePublishResult, error)
	Publish(ctx context.Context, wasmBytes []byte) (*PublishResult, error)
}

type publisher struct {
	server        string
	database      string
	token         string
	clearDatabase bool
	autoConfirm   bool
	client        *http.Client
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

func (p *publisher) WithClearDatabase(clear bool) PublisherBuilder {
	p.clearDatabase = clear
	return p
}

func (p *publisher) WithAutoConfirm(confirm bool) PublisherBuilder {
	p.autoConfirm = confirm
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

func (p *publisher) Publish(ctx context.Context, wasmBytes []byte) (*PublishResult, error) {
	encodedDB := url.PathEscape(p.database)
	u := fmt.Sprintf("%s/v1/database/%s?host_type=Wasm", p.server, encodedDB)

	if p.clearDatabase {
		u += "&clear=true"
	}

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
