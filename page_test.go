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
	cfg.ProxyByType = map[string]string{"codex": "socks5h://pool-a.{account}:secret-token@resin:2260"}
	cfg.OtherProxyURL = "socks5h://only-secret@resin:2260"
	html := renderStatusPage(cfg, "手动同步完成: 写入 1 / 跳过 0 / 未变化 0 / 失败 0", map[string]int{"codex": 3, "gemcli": 1})

	for _, secret := range []string{"secret-token", "only-secret"} {
		if strings.Contains(html, secret) {
			t.Fatalf("token %q leaked into page HTML", secret)
		}
	}
	// per-type inputs must never carry their values server-side
	if strings.Contains(html, "value=\"socks5h") {
		t.Fatalf("proxy entry values must not be server-rendered")
	}
	for _, want := range []string{
		"***", "凭证类型", "other", "rebind_on_change",
		"themeBtn", "pool-a", "主题: 自动",
		"{account}", "基本设置", `id="optEnabled"`,
		`id="selStrategy"`, `id="inpInterval"`, "状态信息",
		"写法说明", "粘性代理（每份凭证独立出口）", "共用代理（整类一个出口）", "全局默认",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("page missing %q", want)
		}
	}
	if strings.Count(html, "data-type=") != len(stickybind.KnownProviders)+2 /* gemcli + other */ {
		t.Fatalf("unexpected row count in page")
	}
}
