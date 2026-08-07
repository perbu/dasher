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

// The endpoint caps per_page at 50, so the count saturates at 50 for a
// very full inbox — fine for a dashboard badge.
const githubNotificationsURL = "https://api.github.com/notifications?per_page=50"

// GitHub periodically fetches the unread notification count.
type GitHub struct {
	cfg    config.GitHubConfig
	store  *state.Store
	client *http.Client
}

// NewGitHub creates the GitHub fetcher.
func NewGitHub(cfg config.GitHubConfig, store *state.Store) *GitHub {
	return &GitHub{
		cfg:    cfg,
		store:  store,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Run starts the periodic fetch loop. It blocks until ctx is cancelled.
func (g *GitHub) Run(ctx context.Context) {
	runLoop(ctx, "github", g.cfg.Interval, g.fetch)
}

func (g *GitHub) fetch(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubNotificationsURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := do("github", g.client, req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	// Only the fields the dashboard needs; the rest of the response is ignored.
	var raw []struct {
		Reason    string    `json:"reason"`
		UpdatedAt time.Time `json:"updated_at"`
		Subject   struct {
			Type string `json:"type"`
		} `json:"subject"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	notifications := make([]state.Notification, 0, len(raw))
	for _, n := range raw {
		notifications = append(notifications, state.Notification{
			Type:      n.Subject.Type,
			Repo:      n.Repository.FullName,
			Reason:    n.Reason,
			UpdatedAt: n.UpdatedAt,
		})
	}

	g.store.SetGitHub(state.GitHubData{
		UnreadNotifications: len(raw),
		Notifications:       notifications,
	})
	return nil
}
