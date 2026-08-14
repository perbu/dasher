package fetchers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"dasher/internal/config"
	"dasher/internal/state"
)

// googleTokenURL exchanges refresh tokens for access tokens. Unlike the
// Anthropic endpoint it wants a form-encoded body, not JSON.
const googleTokenURL = "https://oauth2.googleapis.com/token"

// googleEventsURL lists events on a calendar; %s is the URL-escaped id.
const googleEventsURL = "https://www.googleapis.com/calendar/v3/calendars/%s/events"

// Calendar periodically fetches the next upcoming event from Google Calendar.
// It holds an OAuth refresh token and mints access tokens itself, persisting
// credentials to the token cache the same way the Claude fetcher does — the
// cache outranks the configured seed once written.
type Calendar struct {
	cfg    config.CalendarConfig
	store  *state.Store
	client *http.Client

	mu    sync.Mutex
	creds credentials
}

// NewCalendar creates the Google Calendar fetcher, seeding credentials from
// the token cache when it exists and from config otherwise.
func NewCalendar(cfg config.CalendarConfig, store *state.Store) (*Calendar, error) {
	c := &Calendar{
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
		c.creds = *cached
	}
	if c.creds.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token in calendar.refresh_token or %s", cfg.TokenCache)
	}
	if cfg.ClientSecret == "" {
		return nil, errors.New("calendar.client_secret is required to refresh Google tokens")
	}
	return c, nil
}

// Run starts the periodic fetch loop. It blocks until ctx is cancelled.
func (c *Calendar) Run(ctx context.Context) {
	runLoop(ctx, "calendar", c.cfg.Interval, c.fetch)
}

func (c *Calendar) fetch(ctx context.Context) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	body, status, err := c.getNextEvent(ctx, token)
	if err != nil {
		return err
	}
	// A revoked or prematurely expired token gets one refresh-and-retry
	// before the tick is failed, same as the Claude fetcher.
	if status == http.StatusUnauthorized {
		if token, err = c.refresh(ctx); err != nil {
			return fmt.Errorf("refreshing after 401: %w", err)
		}
		if body, status, err = c.getNextEvent(ctx, token); err != nil {
			return err
		}
	}
	if status != http.StatusOK {
		return fmt.Errorf("unexpected status %d from events endpoint", status)
	}

	var events eventsResponse
	if err := json.Unmarshal(body, &events); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	var d state.CalendarData
	if len(events.Items) > 0 {
		ev := events.Items[0]
		d.Summary = ev.Summary
		if d.Summary == "" {
			// An empty Summary in the payload means "no upcoming event", so an
			// untitled event needs a placeholder to stay distinct from that.
			d.Summary = "(no title)"
		}
		if d.Start, d.End, d.AllDay, err = parseEventTimes(ev.Start, ev.End); err != nil {
			return err
		}
	}
	c.store.SetCalendar(d)
	return nil
}

// eventsResponse is the slice of the events.list document the dashboard
// needs. With maxResults=1 the items array has at most one entry.
type eventsResponse struct {
	Items []struct {
		Summary string     `json:"summary"`
		Start   eventPoint `json:"start"`
		End     eventPoint `json:"end"`
	} `json:"items"`
}

// eventPoint is one end of an event. Timed events carry DateTime (RFC3339
// with offset); all-day events carry Date ("2006-01-02") instead.
type eventPoint struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
}

// parseEventTimes converts the start/end of an event to UTC. All-day events
// become midnight UTC with allDay set — good enough for a "today" marker.
func parseEventTimes(start, end eventPoint) (time.Time, time.Time, bool, error) {
	parse := func(p eventPoint) (time.Time, bool, error) {
		switch {
		case p.DateTime != "":
			t, err := time.Parse(time.RFC3339, p.DateTime)
			if err != nil {
				return time.Time{}, false, fmt.Errorf("parsing event time %q: %w", p.DateTime, err)
			}
			return t.UTC().Truncate(time.Second), false, nil
		case p.Date != "":
			t, err := time.Parse("2006-01-02", p.Date)
			if err != nil {
				return time.Time{}, false, fmt.Errorf("parsing event date %q: %w", p.Date, err)
			}
			return t, true, nil
		}
		return time.Time{}, false, errors.New("event had neither dateTime nor date")
	}
	s, sd, err := parse(start)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("start: %w", err)
	}
	e, _, err := parse(end)
	if err != nil {
		return time.Time{}, time.Time{}, false, fmt.Errorf("end: %w", err)
	}
	return s, e, sd, nil
}

// getNextEvent asks for the single next event and returns the body and
// status. singleEvents expands recurrences into instances and is required
// for orderBy=startTime; timeMin bounds event end, so a meeting in progress
// is still the "next" one.
func (c *Calendar) getNextEvent(ctx context.Context, token string) ([]byte, int, error) {
	q := url.Values{
		"timeMin":      {time.Now().UTC().Format(time.RFC3339)},
		"maxResults":   {"1"},
		"singleEvents": {"true"},
		"orderBy":      {"startTime"},
	}
	u := fmt.Sprintf(googleEventsURL, url.PathEscape(c.cfg.CalendarID)) + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := do("calendar", c.client, req)
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
func (c *Calendar) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	token, expiry := c.creds.AccessToken, c.creds.ExpiresAt
	c.mu.Unlock()

	if token != "" && time.Now().Before(expiry.Add(-refreshSkew)) {
		if Verbose {
			log.Printf("[calendar] access token valid for another %s",
				time.Until(expiry).Round(time.Second))
		}
		return token, nil
	}
	return c.refresh(ctx)
}

// refresh exchanges the refresh token for a new access token and persists
// whatever the endpoint returns.
func (c *Calendar) refresh(ctx context.Context) (string, error) {
	c.mu.Lock()
	refreshToken := c.creds.RefreshToken
	c.mu.Unlock()

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := do("calendar", c.client, req)
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
	// tick trigger its own refresh. Google access tokens live an hour.
	lifetime := time.Duration(tr.ExpiresIn) * time.Second
	if lifetime <= 0 {
		lifetime = time.Hour
	}
	next := credentials{
		AccessToken:  tr.AccessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    time.Now().Add(lifetime),
	}
	// Google rarely rotates refresh tokens, but persist it if one comes back.
	if tr.RefreshToken != "" {
		next.RefreshToken = tr.RefreshToken
	}

	c.mu.Lock()
	c.creds = next
	c.mu.Unlock()

	if Verbose {
		log.Printf("[calendar] access token refreshed, valid %s", lifetime.Round(time.Second))
	}
	if err := writeCredentials(c.cfg.TokenCache, next); err != nil {
		log.Printf("[calendar] WARNING: could not persist credentials to %s: %v", c.cfg.TokenCache, err)
	}
	return tr.AccessToken, nil
}
