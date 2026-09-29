package config

import "testing"

// TestResolveURL 用表驱动用例覆盖相对路径、查询参数和完整 URL。
func TestResolveURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		raw  string
		want string
	}{
		{name: "relative", base: "http://localhost:1200", raw: "/bilibili/user/video/1", want: "http://localhost:1200/bilibili/user/video/1"},
		{name: "relative with query", base: "http://localhost:1200/", raw: "/twitter/user/foo?exclude_rts=1", want: "http://localhost:1200/twitter/user/foo?exclude_rts=1"},
		{name: "absolute", base: "http://localhost:1200", raw: "https://example.com/feed.xml", want: "https://example.com/feed.xml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveURL(tt.base, tt.raw)
			if err != nil {
				t.Fatalf("ResolveURL() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveURLRequiresBaseForRelativePath 验证没有基础地址时拒绝相对 Feed 路径。
func TestResolveURLRequiresBaseForRelativePath(t *testing.T) {
	if _, err := ResolveURL("", "/bilibili/user/video/1"); err == nil {
		t.Fatal("ResolveURL() expected an error for a relative URL without a base")
	}
}

// TestApplyEnvOverrides 验证环境变量可以覆盖对应配置字段。
func TestApplyEnvOverrides(t *testing.T) {
	t.Setenv("OBSIDIANHUB_RSSHUB_BASE_URL", "http://rsshub:1200")
	t.Setenv("OBSIDIANHUB_VAULT_PATH", "/vault")
	t.Setenv("OBSIDIANHUB_DATABASE_PATH", "/data/subhub.db")
	t.Setenv("OBSIDIANHUB_INTERVAL_SECONDS", "60")

	cfg := Config{}
	if err := applyEnvOverrides(&cfg); err != nil {
		t.Fatalf("applyEnvOverrides() error = %v", err)
	}
	if cfg.RSSHubBaseURL != "http://rsshub:1200" || cfg.VaultPath != "/vault" || cfg.DatabasePath != "/data/subhub.db" || cfg.IntervalSecs != 60 {
		t.Fatalf("applyEnvOverrides() = %+v", cfg)
	}
}
