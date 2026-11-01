package stickybind

import (
	"encoding/json"
	"strings"
	"testing"
)

func getenvMap(vals map[string]string) func(string) string {
	return func(k string) string { return vals[k] }
}

func TestDefaultsEnableRebindAndExpand(t *testing.T) {
	cfg := Defaults().Normalize()
	if cfg.RebindOnChange {
		t.Fatal("rebind_on_change should default to false")
	}
	if !cfg.ExpandStickyAccount {
		t.Fatal("expand_sticky_account should default to true")
	}
	if cfg.ProxyByType == nil || cfg.StickyByType == nil {
		t.Fatal("new maps should be non-nil after Normalize")
	}
}

func TestParseConfigNewFields(t *testing.T) {
	req, _ := json.Marshal(map[string]any{
		"config_yaml": "enabled: true\n" +
			"proxy_by_type:\n  codex: socks5h://pool-a:tok1@resin:2260\n  ANTIGRAVITY: http://u:p@proxy.example:8080\n" +
			"sticky_by_type:\n  antigravity: false\n  other: true\n" +
			"other_proxy_url: socks5h://pool-b:tok2@resin:2260\n" +
			"expand_sticky_account: true\nrebind_on_change: false\n",
	})
	cfg := ParseConfigFromRequest(req).Normalize()
	if got := cfg.TypeProxyURL("codex"); got != "socks5h://pool-a:tok1@resin:2260" {
		t.Fatalf("proxy_by_type[codex] = %q", got)
	}
	if got := cfg.TypeProxyURL("antigravity"); got != "http://u:p@proxy.example:8080" {
		t.Fatalf("case-insensitive type lookup failed: %q", got)
	}
	if cfg.StickyForType("antigravity") {
		t.Fatal("sticky_by_type antigravity=false should disable sticky")
	}
	if !cfg.StickyForType("codex") {
		t.Fatal("sticky_by_type missing codex should default true")
	}
	if !cfg.StickyForType(OtherProviderKey) {
		t.Fatal("sticky_by_type other=true should be honoured")
	}
	if cfg.OtherProxyURL != "socks5h://pool-b:tok2@resin:2260" {
		t.Fatalf("other_proxy_url = %q", cfg.OtherProxyURL)
	}
	if cfg.RebindOnChange {
		t.Fatal("rebind_on_change=false should parse")
	}
}

func TestResolveProxyInputPriority(t *testing.T) {
	cfg := Config{
		ProxyByType:   map[string]string{"codex": "socks5h://pool-a:tok1@resin:2260"},
		OtherProxyURL: "socks5h://pool-b:tok2@resin:2260",
	}.Normalize()
	if got := cfg.ResolveProxyInput("codex"); got != cfg.ProxyByType["codex"] {
		t.Fatalf("per-type entry should win: %q", got)
	}
	if got := cfg.ResolveProxyInput("CODEX"); got != cfg.ProxyByType["codex"] {
		t.Fatalf("type lookup should be case-insensitive: %q", got)
	}
	if got := cfg.ResolveProxyInput("gemini"); got != cfg.OtherProxyURL {
		t.Fatalf("unlisted type should fall back to other: %q", got)
	}
	if got := cfg.ResolveProxyInput(""); got != cfg.OtherProxyURL {
		t.Fatalf("empty type should fall back to other: %q", got)
	}
	empty := Config{}.Normalize()
	if got := empty.ResolveProxyInput("codex"); got != "" {
		t.Fatalf("no sources configured should be empty: %q", got)
	}
}

func TestParseTypedProxyURLKeepsIdentity(t *testing.T) {
	proxy, err := ParseTypedProxyURL("socks5h://pool-a:tok1@resin:2260", "RESIN_PROXY_TOKEN", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if proxy.Username != "pool-a" || proxy.Password != "tok1" {
		t.Fatalf("proxy = %#v", proxy)
	}
}

func TestParseTypedProxyURLTokenAsUsername(t *testing.T) {
	proxy, err := ParseTypedProxyURL("socks5h://onlytoken@resin:2260", "RESIN_PROXY_TOKEN", func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if proxy.Username != "" || proxy.Password != "onlytoken" {
		t.Fatalf("username-only form should mean token: %#v", proxy)
	}
}

func TestParseTypedProxyURLEnvFallback(t *testing.T) {
	proxy, err := ParseTypedProxyURL("socks5h://resin:2260", "RESIN_PROXY_TOKEN", getenvMap(map[string]string{"RESIN_PROXY_TOKEN": "envtok"}))
	if err != nil {
		t.Fatal(err)
	}
	if proxy.Username != "" || proxy.Password != "envtok" {
		t.Fatalf("proxy = %#v", proxy)
	}
	if _, err := ParseTypedProxyURL("socks5h://resin:2260", "RESIN_PROXY_TOKEN", func(string) string { return "" }); err == nil {
		t.Fatal("missing token should error")
	}
}

func TestBuildTypedProxyURLExpand(t *testing.T) {
	proxy := ParsedProxy{Scheme: "socks5h", Host: "resin", Port: "2260", Username: "pool-a", Password: "tok1"}
	got, err := BuildTypedProxyURL(proxy, "acc1", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "socks5h://pool-a.acc1:tok1@resin:2260" {
		t.Fatalf("url = %q", got)
	}
}

func TestBuildTypedProxyURLDottedIdentityKept(t *testing.T) {
	proxy := ParsedProxy{Scheme: "socks5h", Host: "resin", Port: "2260", Username: "pool-a.fixed", Password: "tok1"}
	got, err := BuildTypedProxyURL(proxy, "acc1", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "socks5h://pool-a.fixed:tok1@resin:2260" {
		t.Fatalf("dotted identity should stay: %q", got)
	}
}

func TestBuildTypedProxyURLNoExpandPlainProxy(t *testing.T) {
	proxy := ParsedProxy{Scheme: "http", Host: "proxy.example", Port: "8080", Username: "alice", Password: "s3cret"}
	got, err := BuildTypedProxyURL(proxy, "acc1", false)
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://alice:s3cret@proxy.example:8080" {
		t.Fatalf("plain proxy should keep credentials: %q", got)
	}
}

func TestBuildTypedProxyURLNoIdentityTokenOnly(t *testing.T) {
	proxy := ParsedProxy{Scheme: "socks5h", Host: "resin", Port: "2260", Password: "tok1"}
	got, err := BuildTypedProxyURL(proxy, "acc1", true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "socks5h://tok1@resin:2260" {
		t.Fatalf("url = %q", got)
	}
}

func TestBuildTypedProxyURLStickyRequiresAccount(t *testing.T) {
	proxy := ParsedProxy{Scheme: "socks5h", Host: "resin", Port: "2260", Username: "pool-a", Password: "tok1"}
	if _, err := BuildTypedProxyURL(proxy, " ", true); err == nil {
		t.Fatal("dotless identity with expand and no account should error")
	}
}

func TestRedactTypedProxyURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"socks5h://pool-a:tok1@resin:2260", "socks5h://pool-a:***@resin:2260"},
		{"socks5h://onlytoken@resin:2260", "socks5h://***@resin:2260"},
		{"", ""},
	}
	for _, c := range cases {
		got := RedactTypedProxyURL(c.in)
		if got != c.want {
			t.Fatalf("RedactTypedProxyURL(%q) = %q, want %q", c.in, got, c.want)
		}
		if strings.Contains(got, "tok1") || strings.Contains(got, "onlytoken") {
			t.Fatalf("token leaked: %q", got)
		}
	}
}
