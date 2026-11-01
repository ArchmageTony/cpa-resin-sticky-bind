package stickybind

import (
	"encoding/json"
	"testing"
)

func TestParseResinProxyURLFromEnv(t *testing.T) {
	proxy, err := ParseResinProxyURL("socks5h://resin:2260", "RESIN_PROXY_TOKEN", func(k string) string {
		if k == "RESIN_PROXY_TOKEN" {
			return "secret-token"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if proxy.Scheme != "socks5h" || proxy.Host != "resin" || proxy.Port != "2260" || proxy.Password != "secret-token" {
		t.Fatalf("proxy = %#v", proxy)
	}
}

func TestParseResinProxyURLPasswordInURL(t *testing.T) {
	proxy, err := ParseResinProxyURL("socks5h://ignored:tok@resin:2260", "RESIN_PROXY_TOKEN", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if proxy.Password != "tok" {
		t.Fatalf("password = %q", proxy.Password)
	}
	url, err := BuildProxyURL(proxy, "default", "acc1")
	if err != nil {
		t.Fatal(err)
	}
	if url != "socks5h://default.acc1:tok@resin:2260" {
		t.Fatalf("url = %q", url)
	}
	redacted := RedactProxyURL(url)
	if redacted == url || redacted == "" {
		t.Fatalf("redacted = %q", redacted)
	}
	if !contains(redacted, "***") {
		t.Fatalf("redacted missing mask: %q", redacted)
	}
}

func TestParseResinProxyURLTokenAsUsername(t *testing.T) {
	proxy, err := ParseResinProxyURL("socks5h://onlytoken@resin:2260", "RESIN_PROXY_TOKEN", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if proxy.Password != "onlytoken" {
		t.Fatalf("password = %q", proxy.Password)
	}
	url, err := BuildProxyURL(proxy, "default", "acc1")
	if err != nil {
		t.Fatal(err)
	}
	if url != "socks5h://default.acc1:onlytoken@resin:2260" {
		t.Fatalf("url = %q", url)
	}
}

func TestParseConfigFromYAML(t *testing.T) {
	req, _ := json.Marshal(map[string]any{
		"config_yaml": "enabled: true\nresin_proxy_url: socks5h://resin:2260\ndefault_platform: pool-a\nonly_if_empty: true\nsticky_by_type:\n  codex: true\n",
	})
	cfg := ParseConfigFromRequest(req)
	if cfg.DefaultPlatform != "pool-a" || cfg.ResinProxyURL != "socks5h://resin:2260" {
		t.Fatalf("cfg = %#v", cfg)
	}
	// removed fields from earlier iterations must be ignored, not break parsing
	if _, ok := cfg.ProxyByType["codex"]; ok {
		t.Fatalf("unexpected proxy_by_type entry")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return sub == ""
}
