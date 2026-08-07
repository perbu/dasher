// Package state holds the latest aggregated dashboard data.
package state

import (
	"encoding/json"
	"sync"
	"time"
)

// HourForecast describes the weather for a single upcoming hour.
type HourForecast struct {
	Time          string  `json:"time"`
	Temperature   float64 `json:"temperature_c"`
	Precipitation float64 `json:"precipitation_mm"`
	Symbol        string  `json:"symbol"`
}

// Payload is the JSON document published to the MQTT topic.
type Payload struct {
	UpdatedAt time.Time       `json:"updated_at"`
	GitHub    *GitHubData     `json:"github,omitempty"`
	Claude    json.RawMessage `json:"claude,omitempty"`
	Weather   []HourForecast  `json:"weather,omitempty"`
}

// Notification describes a single unread GitHub notification.
type Notification struct {
	Type      string    `json:"type"`
	Repo      string    `json:"repo"`
	Reason    string    `json:"reason"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GitHubData holds the GitHub-derived metrics.
type GitHubData struct {
	UnreadNotifications int            `json:"unread_notifications"`
	Notifications       []Notification `json:"notifications,omitempty"`
}

// Store is a concurrency-safe container for the latest fetched data.
type Store struct {
	mu      sync.RWMutex
	payload Payload
}

// SetGitHub records the latest GitHub metrics.
func (s *Store) SetGitHub(d GitHubData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payload.GitHub = &d
	s.payload.UpdatedAt = time.Now().UTC()
}

// SetClaude records the latest Claude usage document (opaque JSON).
func (s *Store) SetClaude(d json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payload.Claude = d
	s.payload.UpdatedAt = time.Now().UTC()
}

// SetWeather records the latest weather forecast.
func (s *Store) SetWeather(f []HourForecast) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payload.Weather = f
	s.payload.UpdatedAt = time.Now().UTC()
}

// Snapshot returns a copy of the current payload.
func (s *Store) Snapshot() Payload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.payload
}
