package stickybind

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func getenvMap(vals map[string]string) func(string) string {
	return func(k string) string { return vals[k] }
}

func TestDefaultsKeepRebindOffAndMaps(t *testing.T) {
	cfg := Defaults().Normalize()
	if cfg.RebindOnChange {
		t.Fatal("rebind_on_change should default to false")
	}
	if cfg.ProxyByType == nil {
		t.Fatal("proxy_by_type should be non-nil after Normalize")
	}
}

func TestParseConfigNewFields(t *testing.T) {
	req, _ := json.Marshal(map[string]any{
		"config_yaml": "enabled: true\n" +
			"proxy_by_type:\n  codex: socks5h://pool-a.{account}:tok1@resin:2260\n  ANTIGRAVITY: http://u:p@proxy.example:8080\n" +
			"other_proxy_url: socks5h://pool-b:tok2@resin:2260\n" +
			"rebind_on_change: false\n",
	})
	cfg := ParseConfigFromRequest(req).Normalize()
	if got := cfg.TypeProxyURL("codex"); got != "socks5h://pool-a.{account}:tok1@resin:2260" {
		t.Fatalf("proxy_by_type[codex] = %q", got)
	}
	if got := cfg.TypeProxyURL("antigravity"); got != "http://u:p@proxy.example:8080" {
		t.Fatalf("case-insensitive type lookup failed: %q", got)
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

func TestNormalizeProxyEntryAccountBound(t *testing.T) {
	entry, bound, err := NormalizeProxyEntry("socks5h://pool-a.{account}:tok1@resin:2260")
	if err != nil {
		t.Fatal(err)
	}
	if !bound {
		t.Fatal("entry with {account} should be account-bound")
	}
	if entry != "socks5h://pool-a.{account}:tok1@resin:2260" {
		t.Fatalf("entry text must be preserved verbatim: %q", entry)
	}
}

func TestNormalizeProxyEntryVerbatim(t *testing.T) {
	entry, bound, err := NormalizeProxyEntry(" http://alice:s3cret@proxy.example:8080 ")
	if err != nil {
		t.Fatal(err)
	}
	if bound {
		t.Fatal("entry without {account} must not be account-bound")
	}
	if entry != "http://alice:s3cret@proxy.example:8080" {
		t.Fatalf("entry = %q", entry)
	}
}

func TestNormalizeProxyEntryPlaceholderAnywhere(t *testing.T) {
	// the only convention adopted is containment; no position or count limits
	entry, bound, err := NormalizeProxyEntry("socks5h://{account}:{account}@resin:2260")
	if err != nil {
		t.Fatal(err)
	}
	if !bound {
		t.Fatal("multiple placeholders should still be account-bound")
	}
	if entry != "socks5h://{account}:{account}@resin:2260" {
		t.Fatalf("entry = %q", entry)
	}
}

func TestNormalizeProxyEntryRejectsGarbage(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"not a url",
		"ftp://host:2260",
		"socks5h://:2260", // missing host
	}
	for _, c := range cases {
		if _, _, err := NormalizeProxyEntry(c); err == nil {
			t.Fatalf("NormalizeProxyEntry(%q) should fail", c)
		}
	}
}

func TestRenderProxyEntrySubstitutesAll(t *testing.T) {
	got := RenderProxyEntry("socks5h://pool-a.{account}:tok@resin:2260", "acc1")
	if got != "socks5h://pool-a.acc1:tok@resin:2260" {
		t.Fatalf("rendered = %q", got)
	}
	// verbatim entries pass through untouched
	if got := RenderProxyEntry("http://alice:s3cret@proxy.example:8080", "acc1"); got != "http://alice:s3cret@proxy.example:8080" {
		t.Fatalf("verbatim entry changed: %q", got)
	}
}

func TestRenderProxyEntryEscapesNonASCIIAccount(t *testing.T) {
	// auth ids may fall back to non-ASCII file names; the rendered URL must
	// stay parseable and the username must decode back to the account.
	entry := "socks5h://pool-a.{account}:tok@resin:2260"
	rendered := RenderProxyEntry(entry, "nv重授权-linden.boomer4u_y5_icloud.com.json")
	u, err := url.Parse(rendered)
	if err != nil {
		t.Fatalf("rendered URL with non-ASCII account must parse: %v (%q)", err, rendered)
	}
	if got := u.User.Username(); got != "pool-a.nv重授权-linden.boomer4u_y5_icloud.com.json" {
		t.Fatalf("decoded username = %q", got)
	}
	if strings.Contains(rendered, "重授权") {
		t.Fatalf("non-ASCII must be percent-encoded in the stored URL: %q", rendered)
	}
}

func TestRedactTypedProxyURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"socks5h://pool-a:tok1@resin:2260", "socks5h://pool-a:***@resin:2260"},
		{"socks5h://onlytoken@resin:2260", "socks5h://***@resin:2260"},
		// {account} is not URL-safe; the mask must survive it
		{"socks5h://pool-a.{account}:tok1@resin:2260", "socks5h://pool-a.{account}:***@resin:2260"},
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

func TestRedactProxyURLMasksUnparseable(t *testing.T) {
	// raw non-ASCII userinfo breaks url.Parse; the redaction must still mask
	raw := "socks5h://gpt-proxy.nv重授权-x.json:SECRET@resin:2260"
	got := RedactProxyURL(raw)
	if strings.Contains(got, "SECRET") {
		t.Fatalf("token leaked into log output: %q", got)
	}
	if !strings.Contains(got, "***") {
		t.Fatalf("expected mask: %q", got)
	}
}

func TestRedactTypedProxyURLStructuralFallback(t *testing.T) {
	// control characters defeat both the parser and the probe
	raw := "socks5h://bad\x00user:SECRET@resin:2260"
	got := RedactTypedProxyURL(raw)
	if strings.Contains(got, "SECRET") {
		t.Fatalf("token leaked: %q", got)
	}
}

func TestBuildAccountNonASCIIHashed(t *testing.T) {
	cfg := Defaults()
	a := BuildAccount(cfg, AccountInput{AuthID: "NV重授权-linden.boomer4u_y5@icloud.com.json"})
	b := BuildAccount(cfg, AccountInput{AuthID: "NV重授权-linden.boomer4u_y5@icloud.com.json"})
	if a != b {
		t.Fatalf("account derivation must be deterministic: %q vs %q", a, b)
	}
	for _, r := range a {
		if r > 0x7F {
			t.Fatalf("account must be ASCII: %q", a)
		}
	}
	other := BuildAccount(cfg, AccountInput{AuthID: "NV重授权-different@icloud.com.json"})
	if other == a {
		t.Fatalf("distinct inputs must not collapse to the same account: %q", a)
	}
	// plain ASCII inputs keep their exact form (no hash suffix)
	plain := BuildAccount(cfg, AccountInput{AuthID: "plain-id"})
	if plain != "plain-id" {
		t.Fatalf("ASCII account changed: %q", plain)
	}
}
