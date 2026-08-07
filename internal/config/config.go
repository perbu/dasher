// Package config loads service configuration from a YAML file.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration for the service.
type Config struct {
	MQTT    MQTTConfig
	GitHub  GitHubConfig
	Claude  ClaudeConfig
	Yr      YrConfig
	Publish PublishConfig
}

// MQTTConfig configures the embedded broker.
type MQTTConfig struct {
	ListenAddr string
	TLSCert    string
	TLSKey     string
	ClientCA   string
	Topic      string
}

// GitHubConfig configures the GitHub notifications fetcher.
type GitHubConfig struct {
	Token    string
	Interval time.Duration
}

// ClaudeConfig configures the Claude usage fetcher. Access tokens are
// short-lived, so the fetcher authenticates with a refresh token and mints
// access tokens as it needs them.
type ClaudeConfig struct {
	RefreshToken string
	ClientID     string
	TokenURL     string
	TokenCache   string
	URL          string
	Interval     time.Duration
}

// YrConfig configures the Yr.no (MET Norway) weather fetcher.
type YrConfig struct {
	Lat       float64
	Lon       float64
	UserAgent string
	Interval  time.Duration
}

// PublishConfig configures the MQTT publisher.
type PublishConfig struct {
	Interval time.Duration
}

// duration is a time.Duration that unmarshals from a YAML string like "5m".
type duration time.Duration

func (d *duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	*d = duration(parsed)
	return nil
}

// fileConfig mirrors Config for YAML decoding. Secrets may be given as
// ${ENV_VAR} references, which are expanded before parsing.
type fileConfig struct {
	MQTT struct {
		ListenAddr string `yaml:"listen_addr"`
		TLSCert    string `yaml:"tls_cert"`
		TLSKey     string `yaml:"tls_key"`
		ClientCA   string `yaml:"tls_client_ca"`
		Topic      string `yaml:"topic"`
	} `yaml:"mqtt"`
	GitHub struct {
		Token    string   `yaml:"token"`
		Interval duration `yaml:"interval"`
	} `yaml:"github"`
	Claude struct {
		RefreshToken string   `yaml:"refresh_token"`
		ClientID     string   `yaml:"client_id"`
		TokenURL     string   `yaml:"token_url"`
		TokenCache   string   `yaml:"token_cache"`
		URL          string   `yaml:"url"`
		Interval     duration `yaml:"interval"`
	} `yaml:"claude"`
	Yr struct {
		Lat       *float64 `yaml:"lat"`
		Lon       *float64 `yaml:"lon"`
		UserAgent string   `yaml:"user_agent"`
		Interval  duration `yaml:"interval"`
	} `yaml:"yr"`
	Publish struct {
		Interval duration `yaml:"interval"`
	} `yaml:"publish"`
}

// Load reads configuration from the YAML file at path. If requireTLS is
// false (development mode), the mqtt.tls_* values may be omitted and the
// broker will listen without TLS.
func Load(path string, requireTLS bool) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	// Defaults; values present in the file overwrite these.
	fc := fileConfig{}
	fc.MQTT.ListenAddr = ":8883"
	fc.MQTT.Topic = "dashboard/data"
	fc.GitHub.Interval = duration(5 * time.Minute)
	fc.Claude.URL = "https://api.anthropic.com/api/oauth/usage"
	fc.Claude.TokenCache = "claude-token.json"
	fc.Claude.Interval = duration(5 * time.Minute)
	fc.Yr.Interval = duration(15 * time.Minute)
	fc.Publish.Interval = duration(30 * time.Second)

	if err := yaml.Unmarshal([]byte(os.ExpandEnv(string(data))), &fc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	cfg := &Config{
		MQTT: MQTTConfig{
			ListenAddr: fc.MQTT.ListenAddr,
			TLSCert:    fc.MQTT.TLSCert,
			TLSKey:     fc.MQTT.TLSKey,
			ClientCA:   fc.MQTT.ClientCA,
			Topic:      fc.MQTT.Topic,
		},
		GitHub: GitHubConfig{
			Token:    fc.GitHub.Token,
			Interval: time.Duration(fc.GitHub.Interval),
		},
		Claude: ClaudeConfig{
			RefreshToken: fc.Claude.RefreshToken,
			ClientID:     fc.Claude.ClientID,
			TokenURL:     fc.Claude.TokenURL,
			TokenCache:   fc.Claude.TokenCache,
			URL:          fc.Claude.URL,
			Interval:     time.Duration(fc.Claude.Interval),
		},
		Yr: YrConfig{
			UserAgent: fc.Yr.UserAgent,
			Interval:  time.Duration(fc.Yr.Interval),
		},
		Publish: PublishConfig{
			Interval: time.Duration(fc.Publish.Interval),
		},
	}
	if fc.Yr.Lat != nil {
		cfg.Yr.Lat = *fc.Yr.Lat
	}
	if fc.Yr.Lon != nil {
		cfg.Yr.Lon = *fc.Yr.Lon
	}

	var missing []string
	// claude.refresh_token is not required here: once the fetcher has rotated
	// it, the token cache is the authoritative copy and the config seed may be
	// dropped. NewClaude reports the case where neither source has one.
	required := map[string]string{
		"github.token":     cfg.GitHub.Token,
		"claude.client_id": cfg.Claude.ClientID,
		"claude.token_url": cfg.Claude.TokenURL,
		"yr.user_agent":    cfg.Yr.UserAgent,
	}
	if requireTLS {
		required["mqtt.tls_cert"] = cfg.MQTT.TLSCert
		required["mqtt.tls_key"] = cfg.MQTT.TLSKey
		required["mqtt.tls_client_ca"] = cfg.MQTT.ClientCA
	}
	for field, v := range required {
		if v == "" {
			missing = append(missing, field)
		}
	}
	if fc.Yr.Lat == nil {
		missing = append(missing, "yr.lat")
	}
	if fc.Yr.Lon == nil {
		missing = append(missing, "yr.lon")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required config values: %v", missing)
	}
	return cfg, nil
}
