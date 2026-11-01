package main

import (
	"fmt"
	"html"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ArchmageTony/cpa-resin-sticky-bind/internal/stickybind"
)

// renderStatusPage builds the Chinese management page for the plugin. The
// server-rendered HTML never contains proxy tokens: current entries are
// shown redacted, and the browser fills real values only after it
// authenticated against the CPA management API from JS.
func renderStatusPage(cfg stickybind.Config, syncNote string, counts map[string]int) string {
	cfg = cfg.Normalize()
	tokenStatus := "已就绪"
	if _, err := stickybind.ParseResinProxyURL(cfg.ResinProxyURL, cfg.ProxyTokenEnv, os.Getenv); err != nil {
		tokenStatus = "异常: " + err.Error()
	}
	safeURL := stickybind.RedactProxyURL(cfg.ResinProxyURL)
	if u, err := url.Parse(cfg.ResinProxyURL); err == nil && u.User != nil {
		if _, hasPass := u.User.Password(); !hasPass && u.User.Username() != "" {
			safeURL = u.Scheme + "://***@" + u.Host
		}
	}

	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	b.WriteString("<title>Resin Sticky Bind</title>")
	b.WriteString(pageCSS())
	b.WriteString("</head><body>")
	b.WriteString("<div class=\"wrap\">")
	b.WriteString("<header><h1>Resin 粘性代理绑定</h1>")
	b.WriteString("<div class=\"head-actions\"><span class=\"ver\">v" + html.EscapeString(pluginVer) + "</span>")
	b.WriteString("<button id=\"themeBtn\" class=\"ghost\" type=\"button\">主题: 自动</button></div></header>")
	b.WriteString("<p class=\"sub\">为每份凭证生成稳定的 Resin 粘性身份，按凭证类型分流出口代理。</p>")

	if syncNote != "" {
		b.WriteString("<div class=\"note\">" + html.EscapeString(syncNote) + "</div>")
	}

	// ---- status ----
	b.WriteString("<h2>运行状态</h2><table class=\"kv\">")
	kvRow(&b, "插件", pluginID)
	kvRow(&b, "总开关", boolZH(cfg.Enabled))
	kvRow(&b, "全局代理入口", safeURL)
	kvRow(&b, "Token 来源", html.EscapeString(cfg.ProxyTokenEnv)+" 环境变量: "+tokenStatus)
	kvRow(&b, "Account 策略", html.EscapeString(cfg.AccountStrategy))
	kvRow(&b, "同步间隔", fmt.Sprintf("%d 秒", cfg.SyncIntervalSeconds))
	kvRow(&b, "改代理自动跟随", boolZH(cfg.RebindOnChange))
	kvRow(&b, "检查时间", time.Now().Format("2006-01-02 15:04:05"))
	b.WriteString("</table>")

	// ---- per-type form ----
	b.WriteString("<h2>按凭证类型配置代理</h2>")
	b.WriteString("<p class=\"hint\">每行填写一个完整代理地址。Resin 粘性写法: <code>socks5h://平台名:令牌@resin:2260</code>（用户名不含点时自动按凭证补全为 <code>平台名.账号</code>）；用户名已含点则视为完整身份，整类凭证共用。普通代理直接填 <code>http://用户:密码@主机:端口</code> 并取消勾选“粘性”。“other” 是未单独配置类型的兜底入口。</p>")
	b.WriteString("<table class=\"types\" id=\"typeTable\"><thead><tr>")
	b.WriteString("<th>凭证类型</th><th>代理地址</th><th>当前配置</th><th>粘性</th><th>凭证数</th></tr></thead><tbody>")
	for _, row := range pageRows(cfg, counts) {
		renderTypeRow(&b, row)
	}
	b.WriteString("</tbody></table>")

	b.WriteString("<div class=\"opts\">")
	b.WriteString("<label><input type=\"checkbox\" id=\"optRebind\"" + checked(cfg.RebindOnChange) + "> 改动代理后自动重写已绑定凭证（rebind_on_change）</label>")
	b.WriteString("<label><input type=\"checkbox\" id=\"optExpand\"" + checked(cfg.ExpandStickyAccount) + "> 新出现的类型默认启用粘性展开（expand_sticky_account）</label>")
	b.WriteString("</div>")

	b.WriteString("<div class=\"actions\">")
	b.WriteString("<button id=\"saveBtn\" type=\"button\" class=\"primary\">保存配置</button>")
	b.WriteString("<a class=\"btn\" href=\"?action=sync\">立即同步</a>")
	b.WriteString("<button id=\"reloadBtn\" type=\"button\" class=\"ghost\">刷新</button>")
	b.WriteString("</div>")

	// ---- key dialog ----
	b.WriteString("<div id=\"keyBox\" class=\"note hidden\">")
	b.WriteString("<p>保存需要 CPA 管理密钥（与管理面板登录密钥相同）。</p>")
	b.WriteString("<div class=\"keyrow\"><input id=\"keyInput\" type=\"password\" placeholder=\"粘贴管理密钥\" autocomplete=\"off\">")
	b.WriteString("<button id=\"keySave\" type=\"button\" class=\"primary\">确认</button></div>")
	b.WriteString("<p class=\"hint\">密钥只保存在浏览器 sessionStorage，关闭标签页即清除；若管理面板已登录会自动复用其密钥。</p>")
	b.WriteString("</div>")

	b.WriteString("<div id=\"msg\" class=\"note hidden\"></div>")

	b.WriteString("<h2>说明</h2><ul>")
	b.WriteString("<li>保存后 CPA 会自动重载插件配置，随后按同步间隔自动写入各凭证的 <code>proxy_url</code>，也可点“立即同步”。</li>")
	b.WriteString("<li>本页面展示的代理地址始终脱敏（令牌显示为 ***）。</li>")
	b.WriteString("<li>类型清单 = 内置 CPA 支持类型 ∪ 运行时发现的类型；带“发现”标记的是运行时新出现的类型。</li>")
	b.WriteString("<li>未单独配置的类型使用 “other” 行的代理；other 也为空时回退到全局 <code>resin_proxy_url</code> + 平台解析。</li>")
	b.WriteString("</ul>")
	b.WriteString("</div>")
	b.WriteString(pageJS())
	b.WriteString("</body></html>")
	return b.String()
}

// typeRow is one rendered row of the per-type table.
type typeRow struct {
	Provider string // config key; "other" for the fallback row
	Label    string
	Current  string // redacted current proxy or ""
	Sticky   bool
	Count    int
	Discovered bool // seen at runtime but not in the built-in list
	IsOther  bool
}

func pageRows(cfg stickybind.Config, counts map[string]int) []typeRow {
	cfg = cfg.Normalize()
	seen := map[string]bool{}
	rows := make([]typeRow, 0, len(stickybind.KnownProviders)+len(counts)+1)
	add := func(provider string, discovered bool) {
		if provider == "" || seen[provider] || provider == stickybind.OtherProviderKey {
			return
		}
		seen[provider] = true
		current := cfg.TypeProxyURL(provider)
		rows = append(rows, typeRow{
			Provider:   provider,
			Label:      provider,
			Current:    stickybind.RedactTypedProxyURL(current),
			Sticky:     cfg.StickyForType(provider),
			Count:      counts[provider],
			Discovered: discovered,
		})
	}
	for _, p := range stickybind.KnownProviders {
		add(p, false)
	}
	extra := make([]string, 0, len(counts))
	for p := range counts {
		if !seen[p] && p != stickybind.OtherProviderKey {
			extra = append(extra, p)
		}
	}
	sort.Strings(extra)
	for _, p := range extra {
		add(p, true)
	}
	// other row last: dedicated other_proxy_url, sticky flag under "other" key
	otherSticky := true
	if v, ok := cfg.StickyByType[stickybind.OtherProviderKey]; ok {
		otherSticky = stickybind.BoolFromAny(v, true)
	} else {
		otherSticky = cfg.ExpandStickyAccount
	}
	rows = append(rows, typeRow{
		Provider: stickybind.OtherProviderKey,
		Label:    "other",
		Current:  stickybind.RedactTypedProxyURL(cfg.OtherProxyURL),
		Sticky:   otherSticky,
		IsOther:  true,
	})
	return rows
}

func renderTypeRow(b *strings.Builder, row typeRow) {
	b.WriteString("<tr data-type=\"" + html.EscapeString(row.Provider) + "\">")
	b.WriteString("<td class=\"ty\">")
	b.WriteString(html.EscapeString(row.Label))
	if row.IsOther {
		b.WriteString(" <span class=\"badge\">兜底</span>")
	} else if row.Discovered {
		b.WriteString(" <span class=\"badge discover\">发现</span>")
	}
	b.WriteString("</td>")
	b.WriteString("<td><input class=\"px\" type=\"text\" spellcheck=\"false\" autocomplete=\"off\" placeholder=\"未配置，填写代理地址\"></td>")
	b.WriteString("<td class=\"cur\">")
	if row.Current == "" {
		b.WriteString("<span class=\"muted\">未配置</span>")
	} else {
		b.WriteString("<code>" + html.EscapeString(row.Current) + "</code>")
	}
	b.WriteString("</td>")
	b.WriteString("<td><input class=\"sticky\" type=\"checkbox\"" + checked(row.Sticky) + "></td>")
	if row.Count > 0 {
		b.WriteString(fmt.Sprintf("<td>%d 份</td>", row.Count))
	} else {
		b.WriteString("<td class=\"muted\">-</td>")
	}
	b.WriteString("</tr>")
}

func pageCSS() string {
	return `<style>
:root{--bg:#f5f6f8;--card:#ffffff;--fg:#1d2129;--muted:#6b7280;--border:#dfe3e8;--accent:#2563eb;--accent-fg:#fff;--ok:#067d3e;--bad:#b00020;--code-bg:#eef0f3;--note-bg:#e8f1ff;--input-bg:#fff}
html[data-theme="dark"]{--bg:#121316;--card:#1c1e23;--fg:#e7e9ec;--muted:#9aa1ab;--border:#33373d;--accent:#4f8ef7;--accent-fg:#0b0d10;--ok:#4ade80;--bad:#f87171;--code-bg:#26292f;--note-bg:#1d2735;--input-bg:#26292f}
@media (prefers-color-scheme: dark){html:not([data-theme="light"]){--bg:#121316;--card:#1c1e23;--fg:#e7e9ec;--muted:#9aa1ab;--border:#33373d;--accent:#4f8ef7;--accent-fg:#0b0d10;--ok:#4ade80;--bad:#f87171;--code-bg:#26292f;--note-bg:#1d2735;--input-bg:#26292f}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--fg);font-family:system-ui,-apple-system,"PingFang SC","Microsoft YaHei",sans-serif;font-size:14px;line-height:1.6}
.wrap{max-width:960px;margin:0 auto;padding:24px 16px 48px}
header{display:flex;align-items:baseline;justify-content:space-between;gap:12px;flex-wrap:wrap}
h1{font-size:22px;margin:0}h2{font-size:16px;margin:28px 0 8px}
.sub,.hint{color:var(--muted)}code{background:var(--code-bg);padding:1px 6px;border-radius:4px;font-size:12.5px;word-break:break-all}
table{border-collapse:collapse;width:100%;background:var(--card);border:1px solid var(--border);border-radius:8px;overflow:hidden}
th,td{border-bottom:1px solid var(--border);padding:8px 10px;text-align:left;vertical-align:middle}
tbody tr:last-child td{border-bottom:none}
th{color:var(--muted);font-weight:600;font-size:12.5px;background:var(--code-bg)}
td.ty{font-weight:600;white-space:nowrap}
.badge{display:inline-block;font-size:11px;padding:0 6px;border-radius:8px;background:var(--code-bg);color:var(--muted)}
.badge.discover{background:var(--note-bg);color:var(--accent)}
.muted{color:var(--muted)}
input[type=text]{width:100%;min-width:220px;background:var(--input-bg);color:var(--fg);border:1px solid var(--border);border-radius:6px;padding:6px 8px;font-family:ui-monospace,monospace;font-size:12.5px}
input[type=text]:focus{outline:2px solid var(--accent);outline-offset:-1px}
input[type=checkbox]{accent-color:var(--accent)}
.note{margin:12px 0;padding:10px 12px;background:var(--note-bg);border-radius:8px}
.hidden{display:none}
.opts{margin:14px 0;display:flex;flex-direction:column;gap:6px}
.opts label,.keyrow{display:flex;align-items:center;gap:8px}
.actions{display:flex;gap:10px;margin:16px 0;flex-wrap:wrap}
button,.btn{border-radius:6px;border:1px solid var(--border);background:var(--card);color:var(--fg);padding:7px 16px;font-size:14px;cursor:pointer;text-decoration:none;display:inline-block}
button.primary{background:var(--accent);border-color:var(--accent);color:var(--accent-fg);font-weight:600}
button.ghost{background:transparent}
button:disabled{opacity:.5;cursor:not-allowed}
.head-actions{display:flex;align-items:center;gap:10px}
.ver{color:var(--muted);font-size:12.5px}
.keyrow input{flex:1}
</style>`
}

func pageJS() string {
	return `<script>
(function(){
"use strict";
var PLUGIN_ID = "cpa-resin-sticky-bind";
var API = "/v0/management/plugins/" + PLUGIN_ID + "/config";
var THEME_KEY = "resin-sticky-bind-theme";
var SESSION_KEY = "resin-sticky-bind-mgmt-key";
var $ = function(id){ return document.getElementById(id); };
var msgTimer = null;

function showMsg(text, isErr){
  var el = $("msg");
  el.textContent = text;
  el.style.color = isErr ? "var(--bad)" : "var(--ok)";
  el.classList.remove("hidden");
  clearTimeout(msgTimer);
  if (!isErr) msgTimer = setTimeout(function(){ el.classList.add("hidden"); }, 8000);
}

/* ---------- theme: 自动 -> 浅色 -> 深色 ---------- */
var THEME_MODES = ["auto", "light", "dark"];
var THEME_LABEL = {auto: "主题: 自动", light: "主题: 浅色", dark: "主题: 深色"};
function applyTheme(mode){
  if (mode === "light" || mode === "dark") document.documentElement.dataset.theme = mode;
  else delete document.documentElement.dataset.theme;
  $("themeBtn").textContent = THEME_LABEL[mode] || THEME_LABEL.auto;
}
var themeMode = localStorage.getItem(THEME_KEY) || "auto";
if (THEME_MODES.indexOf(themeMode) < 0) themeMode = "auto";
applyTheme(themeMode);
$("themeBtn").addEventListener("click", function(){
  themeMode = THEME_MODES[(THEME_MODES.indexOf(themeMode) + 1) % THEME_MODES.length];
  localStorage.setItem(THEME_KEY, themeMode);
  applyTheme(themeMode);
});
$("reloadBtn").addEventListener("click", function(){ location.reload(); });

/* ---------- management key ---------- */
function candidateKeys(){
  var list = [];
  var s = sessionStorage.getItem(SESSION_KEY);
  if (s) list.push(s);
  try {
    var l = localStorage.getItem("managementKey");
    if (l){
      l = l.trim();
      if (l.charAt(0) === '"' || l.charAt(0) === "{"){
        try { var j = JSON.parse(l); if (typeof j === "string") l = j; else if (j && typeof j.value === "string") l = j.value; } catch(e){}
      }
      if (l) list.push(l);
    }
  } catch(e){}
  return list.filter(function(v, i, a){ return v && a.indexOf(v) === i; });
}
function authHeaders(key){
  return {"Authorization": "Bearer " + key, "X-Management-Key": key};
}
function apiGet(key){
  return fetch(API, {headers: authHeaders(key), cache: "no-store"}).then(function(r){
    if (!r.ok) throw {status: r.status};
    return r.json();
  });
}
var activeKey = null;
function ensureKey(){
  var cands = candidateKeys();
  var chain = Promise.reject();
  cands.forEach(function(k){
    chain = chain.catch(function(){ return apiGet(k).then(function(){ return k; }); });
  });
  return chain.catch(function(){ return promptKey(); });
}
function promptKey(){
  $("keyBox").classList.remove("hidden");
  $("keyInput").focus();
  return new Promise(function(resolve, reject){
    $("keySave").onclick = function(){
      var k = $("keyInput").value.trim();
      if (!k) return;
      apiGet(k).then(function(){
        try { sessionStorage.setItem(SESSION_KEY, k); } catch(e){}
        $("keyBox").classList.add("hidden");
        $("keyInput").value = "";
        resolve(k);
      }).catch(function(){ showMsg("密钥校验失败，请重试", true); reject(new Error("bad key")); });
    };
  });
}

/* ---------- load & fill ---------- */
var configCache = null;
function markDirty(input){
  input.dataset.dirty = "1";
}
document.querySelectorAll("#typeTable tr[data-type]").forEach(function(tr){
  var input = tr.querySelector("input.px");
  input.addEventListener("input", function(){ markDirty(input); });
});
function fillForm(cfg){
  configCache = cfg || {};
  var byType = configCache.proxy_by_type || {};
  var sticky = configCache.sticky_by_type || {};
  document.querySelectorAll("#typeTable tr[data-type]").forEach(function(tr){
    var t = tr.dataset.type;
    var input = tr.querySelector("input.px");
    var stickyBox = tr.querySelector("input.sticky");
    var current = (t === "other") ? (configCache.other_proxy_url || "") : (byType[t] || "");
    if (current){
      input.value = current;
      input.dataset.origin = current;
    }
    if (sticky.hasOwnProperty(t)) stickyBox.checked = !!sticky[t];
  });
  if (typeof configCache.rebind_on_change === "boolean") $("optRebind").checked = configCache.rebind_on_change;
  if (typeof configCache.expand_sticky_account === "boolean") $("optExpand").checked = configCache.expand_sticky_account;
}
ensureKey().then(function(k){ activeKey = k; return apiGet(k); }).then(fillForm).catch(function(){ /* user can still retry on save */ });

/* ---------- save ---------- */
$("saveBtn").addEventListener("click", function(){
  var btn = this;
  var body = {proxy_by_type: {}, sticky_by_type: {}, other_proxy_url: null};
  var typed = body.proxy_by_type;
  var sticky = body.sticky_by_type;
  var prevTyped = (configCache && configCache.proxy_by_type) || {};
  var prevSticky = (configCache && configCache.sticky_by_type) || {};
  // keep untouched rows so PATCH of the whole map does not drop them
  Object.keys(prevTyped).forEach(function(k){ typed[k] = prevTyped[k]; });
  Object.keys(prevSticky).forEach(function(k){ sticky[k] = prevSticky[k]; });
  var changed = false;
  document.querySelectorAll("#typeTable tr[data-type]").forEach(function(tr){
    var t = tr.dataset.type;
    var input = tr.querySelector("input.px");
    var val = input.value.trim();
    var origin = input.dataset.origin || "";
    if (val && val !== origin){
      if (t === "other") body.other_proxy_url = val; else typed[t] = val;
      changed = true;
    } else if (!val && origin && input.dataset.dirty){
      // cleared on purpose
      if (t === "other") body.other_proxy_url = null; else delete typed[t];
      changed = true;
    }
    var flag = tr.querySelector("input.sticky").checked;
    if (t === "other"){
      if ((prevSticky.other ? !!prevSticky.other : true) !== flag){ sticky.other = flag; changed = true; }
    } else if (prevTyped[t] || val){
      if ((prevSticky.hasOwnProperty(t) ? !!prevSticky[t] : true) !== flag){ sticky[t] = flag; changed = true; }
    }
  });
  if (($("optRebind").checked) !== ((configCache && typeof configCache.rebind_on_change === "boolean") ? configCache.rebind_on_change : false)){ body.rebind_on_change = $("optRebind").checked; changed = true; }
  if (($("optExpand").checked) !== ((configCache && typeof configCache.expand_sticky_account === "boolean") ? configCache.expand_sticky_account : true)){ body.expand_sticky_account = $("optExpand").checked; changed = true; }
  if (!changed){ showMsg("没有需要保存的改动"); return; }
  // drop null other_proxy_url when it was already absent
  if (body.other_proxy_url === null && !(configCache && configCache.other_proxy_url)) delete body.other_proxy_url;
  if (body.other_proxy_url === null){ /* explicit delete is fine for PATCH */ }
  btn.disabled = true;
  doSave(body).then(function(){
    showMsg("已保存，CPA 正在重载配置，页面稍后自动刷新…");
    setTimeout(function(){ location.reload(); }, 2500);
  }).catch(function(err){
    btn.disabled = false;
    showMsg("保存失败: " + (err && err.message ? err.message : "未知错误"), true);
  });
});
function doSave(body){
  var run = function(key){
    activeKey = key;
    return fetch(API, {method: "PATCH", headers: Object.assign({"Content-Type": "application/json"}, authHeaders(key)), body: JSON.stringify(body)}).then(function(r){
      if (r.status === 401 || r.status === 403) throw {needKey: true};
      if (!r.ok) throw new Error("HTTP " + r.status);
      try { sessionStorage.setItem(SESSION_KEY, key); } catch(e){}
    });
  };
  var key = activeKey || candidateKeys()[0] || "";
  return run(key).catch(function(err){
    if (err && err.needKey){
      sessionStorage.removeItem(SESSION_KEY);
      activeKey = null;
      return promptKey().then(run);
    }
    throw err;
  });
}
})();
</script>`
}

func kvRow(b *strings.Builder, k, v string) {
	b.WriteString("<tr><th>" + html.EscapeString(k) + "</th><td>" + v + "</td></tr>")
}

func boolZH(v bool) string {
	if v {
		return "开启"
	}
	return "关闭"
}

func checked(v bool) string {
	if v {
		return " checked"
	}
	return ""
}
