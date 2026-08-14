// Package state holds the latest aggregated dashboard data.
package state

import (
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
	UpdatedAt time.Time      `json:"updated_at"`
	GitHub    *GitHubData    `json:"github,omitempty"`
	Claude    *ClaudeData    `json:"claude,omitempty"`
	Weather   []HourForecast `json:"weather,omitempty"`
	Presence  *PresenceData  `json:"presence,omitempty"`
	Calendar  *CalendarData  `json:"calendar,omitempty"`
}

// CalendarData describes the next upcoming calendar event. A successful fetch
// always sets it: an empty Summary means there is no upcoming event, while an
// absent calendar section means the fetcher has not produced data yet.
type CalendarData struct {
	Summary string    `json:"summary"`
	Start   time.Time `json:"start,omitempty"`
	End     time.Time `json:"end,omitempty"`
	AllDay  bool      `json:"all_day,omitempty"`
}

// PresenceData reports whether the owner is on the network. The device turns
// its screen off while Home is false.
type PresenceData struct {
	Home bool `json:"home"`
}

// ClaudeData holds the Claude usage metrics the dashboard renders. The usage
// endpoint returns a far larger document; only these fields are published, to
// keep the payload small enough for the ESP32.
type ClaudeData struct {
	FiveHourPct      float64   `json:"five_hour_pct"`
	FiveHourResetsAt time.Time `json:"five_hour_resets_at"`
	SevenDayPct      float64   `json:"seven_day_pct"`
	SevenDayResetsAt time.Time `json:"seven_day_resets_at"`
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

// SetClaude records the latest Claude usage metrics.
func (s *Store) SetClaude(d ClaudeData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payload.Claude = &d
	s.payload.UpdatedAt = time.Now().UTC()
}

// SetWeather records the latest weather forecast.
func (s *Store) SetWeather(f []HourForecast) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payload.Weather = f
	s.payload.UpdatedAt = time.Now().UTC()
}

// SetPresence records the latest presence check result.
func (s *Store) SetPresence(home bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payload.Presence = &PresenceData{Home: home}
	s.payload.UpdatedAt = time.Now().UTC()
}

// SetCalendar records the next upcoming calendar event.
func (s *Store) SetCalendar(d CalendarData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payload.Calendar = &d
	s.payload.UpdatedAt = time.Now().UTC()
}

// Snapshot returns a copy of the current payload.
func (s *Store) Snapshot() Payload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.payload
}
