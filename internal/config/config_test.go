package config

import "testing"

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

func TestResolveURLRequiresBaseForRelativePath(t *testing.T) {
	if _, err := ResolveURL("", "/bilibili/user/video/1"); err == nil {
		t.Fatal("ResolveURL() expected an error for a relative URL without a base")
	}
}
