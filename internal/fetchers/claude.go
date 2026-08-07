package fetchers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"dasher/internal/config"
	"dasher/internal/state"
)

// Claude periodically fetches usage/quota metrics from the Anthropic API.
// The response is stored as opaque JSON so dashboard clients can render
// whatever fields the API currently returns.
type Claude struct {
	cfg    config.ClaudeConfig
	store  *state.Store
	client *http.Client
}

// NewClaude creates the Claude usage fetcher.
func NewClaude(cfg config.ClaudeConfig, store *state.Store) *Claude {
	return &Claude{
		cfg:    cfg,
		store:  store,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Run starts the periodic fetch loop. It blocks until ctx is cancelled.
func (c *Claude) Run(ctx context.Context) {
	runLoop(ctx, "claude", c.cfg.Interval, c.fetch)
}

func (c *Claude) fetch(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("User-Agent", "claude-code/2.1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	// Validate it is JSON before storing.
	var raw json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	c.store.SetClaude(raw)
	return nil
}
