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

// Config 保存程序运行参数；JSON 标签定义配置文件中的字段名。
type Config struct {
	RSSHubBaseURL string         `json:"rsshub_base_url"`  // 相对 Feed URL 使用的 RSSHub 根地址。
	VaultPath     string         `json:"vault_path"`       // Obsidian Vault 的本地目录。
	DatabasePath  string         `json:"database_path"`    // SQLite 去重数据库文件路径。
	IntervalSecs  int            `json:"interval_seconds"` // 常驻模式下两轮同步之间的秒数。
	UserAgent     string         `json:"user_agent"`       // 拉取 Feed 时发送的 HTTP User-Agent。
	Subscriptions []Subscription `json:"subscriptions"`    // 需要轮询的 Feed 列表。
}

// Subscription 描述一个需要轮询的 Feed，以及笔记在 Vault 中的归档方式。
type Subscription struct {
	ID      string   `json:"id"`      // 稳定去重标识；为空时由 Normalize 根据来源和 URL 生成。
	Name    string   `json:"name"`    // 用于日志和错误信息展示的名称。
	Source  string   `json:"source"`  // 来源标记，写入笔记 frontmatter。
	URL     string   `json:"url"`     // 完整 Feed URL 或相对 RSSHub 的路由。
	Folder  string   `json:"folder"`  // Vault 内的归档子目录。
	Tags    []string `json:"tags"`    // 写入笔记 frontmatter 的标签。
	Enabled bool     `json:"enabled"` // false 时跳过该订阅。
}

// Load 读取 JSON 配置，应用环境变量覆盖，并执行默认值填充和合法性检查。
func Load(path string) (Config, error) {
	// ReadFile 读取整个小型配置文件；Unmarshal 根据字段上的 json 标签填充 Config。
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

// applyEnvOverrides 让部署环境可以覆盖文件配置；空环境变量视为“未设置”。
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
		// Atoi 负责字符串到整数的转换，随后显式拒绝非正数。
		seconds, err := strconv.Atoi(value)
		if err != nil || seconds <= 0 {
			return fmt.Errorf("OBSIDIANHUB_INTERVAL_SECONDS must be a positive integer, got %q", value)
		}
		c.IntervalSecs = seconds
	}
	return nil
}

// Normalize 填补默认值并校验订阅配置；它会直接修改接收者中的字段。
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
			// 没有显式 ID 时，根据来源和 URL 生成稳定 ID；NUL 用来避免简单拼接歧义。
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

// ResolveURL 将 Feed 地址解析为绝对 URL：完整 URL 原样规范化返回，相对路径拼到 RSSHub 基址。
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
	// 只组合 Path，并单独保留 RawQuery，确保查询参数不会被误当作路径内容。
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + path
	baseURL.RawQuery = u.RawQuery
	return baseURL.String(), nil
}

// expandHome 把 ~ 或 ~/ 开头的本地路径展开为用户主目录；其他路径保持原样。
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
