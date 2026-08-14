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

const yrBaseURL = "https://api.met.no/weatherapi/locationforecast/2.0/compact"

// Yr periodically fetches the weather forecast for the next 3 hours from
// MET Norway (api.met.no, the data source behind Yr.no). The display only
// has room for three columns since the meeting card took the rest of the
// strip.
type Yr struct {
	cfg    config.YrConfig
	store  *state.Store
	client *http.Client
}

// NewYr creates the Yr.no fetcher.
func NewYr(cfg config.YrConfig, store *state.Store) *Yr {
	return &Yr{
		cfg:    cfg,
		store:  store,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Run starts the periodic fetch loop. It blocks until ctx is cancelled.
func (y *Yr) Run(ctx context.Context) {
	runLoop(ctx, "yr", y.cfg.Interval, y.fetch)
}

// locationforecast compact response structure (subset of fields we use).
type yrResponse struct {
	Properties struct {
		Timeseries []struct {
			Time time.Time `json:"time"`
			Data struct {
				Instant struct {
					Details struct {
						AirTemperature float64 `json:"air_temperature"`
					} `json:"details"`
				} `json:"instant"`
				Next1Hours *struct {
					Summary struct {
						SymbolCode string `json:"symbol_code"`
					} `json:"summary"`
					Details struct {
						PrecipitationAmount float64 `json:"precipitation_amount"`
					} `json:"details"`
				} `json:"next_1_hours"`
				Next6Hours *struct {
					Summary struct {
						SymbolCode string `json:"symbol_code"`
					} `json:"summary"`
				} `json:"next_6_hours"`
			} `json:"data"`
		} `json:"timeseries"`
	} `json:"properties"`
}

func (y *Yr) fetch(ctx context.Context) error {
	url := fmt.Sprintf("%s?lat=%.4f&lon=%.4f", yrBaseURL, y.cfg.Lat, y.cfg.Lon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// MET Norway requires an identifying User-Agent.
	req.Header.Set("User-Agent", y.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := do("yr", y.client, req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var forecast yrResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&forecast); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	now := time.Now().UTC()
	hours := make([]state.HourForecast, 0, 3)
	for _, ts := range forecast.Properties.Timeseries {
		if ts.Time.Before(now) {
			continue
		}
		h := state.HourForecast{
			Time:        ts.Time.Format(time.RFC3339),
			Temperature: ts.Data.Instant.Details.AirTemperature,
		}
		if n := ts.Data.Next1Hours; n != nil {
			h.Symbol = n.Summary.SymbolCode
			h.Precipitation = n.Details.PrecipitationAmount
		} else if n := ts.Data.Next6Hours; n != nil {
			h.Symbol = n.Summary.SymbolCode
		}
		hours = append(hours, h)
		if len(hours) == 3 {
			break
		}
	}
	if len(hours) == 0 {
		return fmt.Errorf("no future forecast entries in response")
	}

	y.store.SetWeather(hours)
	return nil
}
