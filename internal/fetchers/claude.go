package fetchers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dasher/internal/config"
	"dasher/internal/state"
)

// refreshSkew is how far ahead of expiry the access token is renewed.
const refreshSkew = 5 * time.Minute

// claudeUserAgent identifies dasher honestly. Do not put Claude Code's own
// User-Agent here: the OAuth token endpoint answers 404 to requests claiming
// to be `claude-code/<version>`, and the usage endpoint serves us either way,
// so impersonating it buys nothing and breaks token refresh.
const claudeUserAgent = "dasher/1.0"

// Claude periodically fetches usage/quota metrics from the Anthropic API and
// projects them down to the handful of fields the dashboard renders.
//
// OAuth access tokens live only a few hours, so the fetcher holds a refresh
// token and exchanges it for a new access token as the current one nears
// expiry. The exchange may hand back a rotated refresh token, so credentials
// are persisted to the token cache and preferred over the configured seed.
type Claude struct {
	cfg    config.ClaudeConfig
	store  *state.Store
	client *http.Client

	mu    sync.Mutex
	creds credentials
}

// credentials is the OAuth state, as persisted to the token cache.
type credentials struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// NewClaude creates the Claude usage fetcher, seeding credentials from the
// token cache when it exists and from config otherwise.
func NewClaude(cfg config.ClaudeConfig, store *state.Store) (*Claude, error) {
	c := &Claude{
		cfg:    cfg,
		store:  store,
		client: &http.Client{Timeout: 15 * time.Second},
		creds:  credentials{RefreshToken: cfg.RefreshToken},
	}
	cached, err := readCredentials(cfg.TokenCache)
	if err != nil {
		return nil, err
	}
	if cached != nil {
		// A rotated refresh token outranks the configured seed, which goes
		// stale the first time the endpoint issues a replacement.
		c.creds = *cached
	}
	if c.creds.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token in claude.refresh_token or %s", cfg.TokenCache)
	}
	return c, nil
}

// Run starts the periodic fetch loop. It blocks until ctx is cancelled.
func (c *Claude) Run(ctx context.Context) {
	runLoop(ctx, "claude", c.cfg.Interval, c.fetch)
}

func (c *Claude) fetch(ctx context.Context) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	body, status, err := c.getUsage(ctx, token)
	if err != nil {
		return err
	}
	// A token can be revoked server-side well before its recorded expiry, so
	// treat one 401 as a hint to refresh rather than a failed tick.
	if status == http.StatusUnauthorized {
		if token, err = c.refresh(ctx); err != nil {
			return fmt.Errorf("refreshing after 401: %w", err)
		}
		if body, status, err = c.getUsage(ctx, token); err != nil {
			return err
		}
	}
	if status != http.StatusOK {
		return fmt.Errorf("unexpected status %d from usage endpoint", status)
	}

	var u usageResponse
	if err := json.Unmarshal(body, &u); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	// Absent reset times mean the fields were renamed rather than that usage is
	// genuinely zero. Fail the tick so the log shows it instead of publishing
	// a plausible-looking 0%.
	if u.FiveHour.ResetsAt.IsZero() && u.SevenDay.ResetsAt.IsZero() {
		return errors.New("usage response had no reset times; endpoint schema may have changed")
	}

	c.store.SetClaude(state.ClaudeData{
		FiveHourPct:      u.FiveHour.Utilization,
		FiveHourResetsAt: u.FiveHour.ResetsAt.UTC().Truncate(time.Second),
		SevenDayPct:      u.SevenDay.Utilization,
		SevenDayResetsAt: u.SevenDay.ResetsAt.UTC().Truncate(time.Second),
	})
	return nil
}

// usageResponse is the slice of the usage document that the dashboard needs.
// The endpoint returns much more — per-model breakdowns, spend, and a set of
// unreleased feature flags — but its schema is not a stable contract, so only
// the fields actually rendered are modelled here.
type usageResponse struct {
	FiveHour usageWindow `json:"five_hour"`
	SevenDay usageWindow `json:"seven_day"`
}

// usageWindow is one rate-limit window. Note the dollar figures the endpoint
// also returns are null on Team plans, so utilization is the only usable
// measure across subscription types.
type usageWindow struct {
	Utilization float64   `json:"utilization"`
	ResetsAt    time.Time `json:"resets_at"`
}

// getUsage calls the usage endpoint and returns the body and status. A non-OK
// status is reported to the caller rather than treated as an error, so fetch
// can decide whether it is worth retrying.
func (c *Claude) getUsage(ctx context.Context, token string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.URL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("User-Agent", claudeUserAgent)

	resp, err := do("claude", c.client, req)
	if err != nil {
		return nil, 0, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, 0, fmt.Errorf("reading response: %w", err)
	}
	return body, resp.StatusCode, nil
}

// accessToken returns a usable access token, refreshing when the current one
// is missing or close to expiring.
func (c *Claude) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	token, expiry := c.creds.AccessToken, c.creds.ExpiresAt
	c.mu.Unlock()

	if token != "" && time.Now().Before(expiry.Add(-refreshSkew)) {
		if Verbose {
			log.Printf("[claude] access token valid for another %s",
				time.Until(expiry).Round(time.Second))
		}
		return token, nil
	}
	if Verbose {
		switch {
		case token == "":
			log.Print("[claude] no access token cached, refreshing")
		default:
			log.Printf("[claude] access token expires %s, refreshing",
				time.Until(expiry).Round(time.Second))
		}
	}
	return c.refresh(ctx)
}

// refresh exchanges the refresh token for a new access token and persists
// whatever the endpoint returns.
func (c *Claude) refresh(ctx context.Context) (string, error) {
	c.mu.Lock()
	refreshToken := c.creds.RefreshToken
	c.mu.Unlock()

	reqBody, err := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     c.cfg.ClientID,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", claudeUserAgent)

	resp, err := do("claude", c.client, req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", fmt.Errorf("reading token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, body)
	}

	var tr struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", errors.New("token response contained no access token")
	}

	// Guard against a missing expires_in, which would otherwise make every
	// tick trigger its own refresh.
	lifetime := time.Duration(tr.ExpiresIn) * time.Second
	if lifetime <= 0 {
		lifetime = time.Hour
	}
	next := credentials{
		AccessToken:  tr.AccessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(lifetime),
	}
	if tr.RefreshToken != "" {
		next.RefreshToken = tr.RefreshToken
	}

	c.mu.Lock()
	c.creds = next
	c.mu.Unlock()

	// Logged unconditionally: this runs roughly every six hours, and a
	// rotation is the moment any other holder of the old refresh token
	// (Claude Code included) stops working.
	rotated := ""
	if next.RefreshToken != refreshToken {
		rotated = "; refresh token rotated"
	}
	log.Printf("[claude] access token refreshed, valid %s (until %s)%s",
		lifetime.Round(time.Second), next.ExpiresAt.Format(time.RFC3339), rotated)

	if err := writeCredentials(c.cfg.TokenCache, next); err != nil {
		// The in-memory token still works, but losing a rotated refresh token
		// means the next restart cannot authenticate at all.
		log.Printf("[claude] WARNING: could not persist credentials to %s: %v", c.cfg.TokenCache, err)
	}
	return tr.AccessToken, nil
}

// readCredentials loads the token cache. A missing file is not an error.
func readCredentials(path string) (*credentials, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading token cache: %w", err)
	}
	var c credentials
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing token cache %s: %w", path, err)
	}
	if c.RefreshToken == "" {
		return nil, fmt.Errorf("token cache %s has no refresh token", path)
	}
	return &c, nil
}

// writeCredentials persists credentials with owner-only permissions, writing
// through a temporary file so an interrupted run cannot leave a truncated
// cache in place of a working refresh token.
func writeCredentials(path string, c credentials) error {
	if path == "" {
		return nil
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".claude-token-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once the rename succeeds
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
