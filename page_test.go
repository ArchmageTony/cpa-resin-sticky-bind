package main

import (
	"strings"
	"testing"

	"github.com/ArchmageTony/cpa-resin-sticky-bind/internal/stickybind"
)

// The page is served without CPA management auth, so rendered HTML must
// never contain raw tokens and must expose the theme toggle.
func TestRenderStatusPageRedactsTokens(t *testing.T) {
	cfg := stickybind.Defaults()
	cfg.ProxyByType = map[string]string{"codex": "socks5h://pool-a:secret-token@resin:2260"}
	cfg.OtherProxyURL = "socks5h://only-secret@resin:2260"
	cfg.StickyByType = map[string]string{"codex": "true"}
	html := renderStatusPage(cfg, "手动同步完成: 写入 1 / 跳过 0 / 未变化 0 / 失败 0", map[string]int{"codex": 3, "gemcli": 1})

	for _, secret := range []string{"secret-token", "only-secret"} {
		if strings.Contains(html, secret) {
			t.Fatalf("token %q leaked into page HTML", secret)
		}
	}
	for _, want := range []string{"***", "凭证类型", "other", "rebind_on_change", "themeBtn", "pool-a", "主题: 自动"} {
		if !strings.Contains(html, want) {
			t.Fatalf("page missing %q", want)
		}
	}
	if strings.Count(html, "data-type=") != len(stickybind.KnownProviders)+2 /* gemcli + other */ {
		t.Fatalf("unexpected row count in page")
	}
}
