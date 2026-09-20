package config

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	RSSHubBaseURL string         `json:"rsshub_base_url"`
	VaultPath     string         `json:"vault_path"`
	DatabasePath  string         `json:"database_path"`
	IntervalSecs  int            `json:"interval_seconds"`
	UserAgent     string         `json:"user_agent"`
	Subscriptions []Subscription `json:"subscriptions"`
}

type Subscription struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Source  string   `json:"source"`
	URL     string   `json:"url"`
	Folder  string   `json:"folder"`
	Tags    []string `json:"tags"`
	Enabled bool     `json:"enabled"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := applyEnvOverrides(&cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Normalize(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyEnvOverrides(c *Config) error {
	if value := strings.TrimSpace(os.Getenv("OBSIDIANHUB_RSSHUB_BASE_URL")); value != "" {
		c.RSSHubBaseURL = value
	}
	if value := strings.TrimSpace(os.Getenv("OBSIDIANHUB_VAULT_PATH")); value != "" {
		c.VaultPath = value
	}
	if value := strings.TrimSpace(os.Getenv("OBSIDIANHUB_DATABASE_PATH")); value != "" {
		c.DatabasePath = value
	}
	if value := strings.TrimSpace(os.Getenv("OBSIDIANHUB_USER_AGENT")); value != "" {
		c.UserAgent = value
	}
	if value := strings.TrimSpace(os.Getenv("OBSIDIANHUB_INTERVAL_SECONDS")); value != "" {
		seconds, err := strconv.Atoi(value)
		if err != nil || seconds <= 0 {
			return fmt.Errorf("OBSIDIANHUB_INTERVAL_SECONDS must be a positive integer, got %q", value)
		}
		c.IntervalSecs = seconds
	}
	return nil
}

func (c *Config) Normalize() error {
	if strings.TrimSpace(c.VaultPath) == "" {
		return errors.New("vault_path is required")
	}
	c.VaultPath = expandHome(c.VaultPath)

	if c.DatabasePath == "" {
		c.DatabasePath = "./data/subhub.db"
	}
	c.DatabasePath = expandHome(c.DatabasePath)

	if c.IntervalSecs <= 0 {
		c.IntervalSecs = 300
	}
	if c.UserAgent == "" {
		c.UserAgent = "obsidianhub/0.1"
	}
	c.RSSHubBaseURL = strings.TrimRight(strings.TrimSpace(c.RSSHubBaseURL), "/")

	seen := make(map[string]struct{}, len(c.Subscriptions))
	for i := range c.Subscriptions {
		s := &c.Subscriptions[i]
		s.Source = strings.TrimSpace(s.Source)
		s.URL = strings.TrimSpace(s.URL)
		s.Name = strings.TrimSpace(s.Name)
		if s.URL == "" {
			return fmt.Errorf("subscription %d: url is required", i)
		}
		if s.Name == "" {
			s.Name = s.Source
		}
		if s.ID == "" {
			h := sha1.Sum([]byte(s.Source + "\x00" + s.URL))
			s.ID = "sub-" + hex.EncodeToString(h[:])[:12]
		}
		if _, ok := seen[s.ID]; ok {
			return fmt.Errorf("duplicate subscription id: %s", s.ID)
		}
		seen[s.ID] = struct{}{}
		if !s.Enabled {
			continue
		}
		if _, err := ResolveURL(c.RSSHubBaseURL, s.URL); err != nil {
			return fmt.Errorf("subscription %s: %w", s.ID, err)
		}
	}
	return nil
}

func ResolveURL(base, raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid feed url %q: %w", raw, err)
	}
	if u.IsAbs() && u.Host != "" {
		return u.String(), nil
	}
	if strings.TrimSpace(base) == "" {
		return "", fmt.Errorf("relative feed url %q requires rsshub_base_url", raw)
	}
	baseURL, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || baseURL.Host == "" {
		return "", fmt.Errorf("invalid rsshub_base_url %q", base)
	}
	path := "/" + strings.TrimLeft(u.Path, "/")
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + path
	baseURL.RawQuery = u.RawQuery
	return baseURL.String(), nil
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}
