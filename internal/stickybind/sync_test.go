package stickybind

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

type mockHost struct {
	entries []HostAuthEntry
	files   map[string]json.RawMessage // auth_index -> json
	names   map[string]string
	saved   map[string]json.RawMessage
	logs    []string
}

func (m *mockHost) ListAuths() ([]HostAuthEntry, error) { return m.entries, nil }
func (m *mockHost) GetAuth(authIndex string) (AuthGetResult, error) {
	raw, ok := m.files[authIndex]
	if !ok {
		return AuthGetResult{}, fmt.Errorf("missing %s", authIndex)
	}
	return AuthGetResult{
		AuthIndex: authIndex,
		Name:      m.names[authIndex],
		JSON:      raw,
	}, nil
}
func (m *mockHost) SaveAuth(name string, rawJSON json.RawMessage) error {
	if m.saved == nil {
		m.saved = map[string]json.RawMessage{}
	}
	m.saved[name] = append(json.RawMessage(nil), rawJSON...)
	// update underlying by name match
	for idx, n := range m.names {
		if n == name {
			m.files[idx] = append(json.RawMessage(nil), rawJSON...)
		}
	}
	return nil
}
func (m *mockHost) Log(level, message string) {
	m.logs = append(m.logs, level+":"+message)
}

func TestSyncOnceWritesEmptyProxy(t *testing.T) {
	host := &mockHost{
		entries: []HostAuthEntry{{
			ID:        "xai-a.json",
			AuthIndex: "idx1",
			Name:      "xai-a.json",
			Provider:  "xai",
			Email:     "a@example.com",
		}},
		files: map[string]json.RawMessage{
			"idx1": json.RawMessage(`{"type":"xai","email":"a@example.com","access_token":"t"}`),
		},
		names: map[string]string{"idx1": "xai-a.json"},
	}
	b := &Binder{
		Cfg: Defaults(),
		Host: host,
		Getenv: func(string) string { return "tok" },
	}
	wrote, skipped, unchanged, failed, err := b.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 1 || skipped != 0 || unchanged != 0 || failed != 0 {
		t.Fatalf("stats wrote=%d skipped=%d unchanged=%d failed=%d", wrote, skipped, unchanged, failed)
	}
	raw := host.saved["xai-a.json"]
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	proxy, _ := meta["proxy_url"].(string)
	wantPrefix := "socks5h://default."
	if proxy == "" || len(proxy) < len(wantPrefix) || proxy[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("proxy = %q, want prefix %q", proxy, wantPrefix)
	}
	if RedactProxyURL(proxy) == proxy {
		t.Fatalf("should redact: %q", proxy)
	}
}

func TestSyncOnceSkipsExistingProxy(t *testing.T) {
	host := &mockHost{
		entries: []HostAuthEntry{{
			ID: "xai-b.json", AuthIndex: "idx2", Name: "xai-b.json", Provider: "xai",
		}},
		files: map[string]json.RawMessage{
			"idx2": json.RawMessage(`{"type":"xai","proxy_url":"socks5h://default.custom:tok@resin:2260"}`),
		},
		names: map[string]string{"idx2": "xai-b.json"},
	}
	cfg := Defaults()
	cfg.RebindOnChange = false // v0.1 behaviour: never touch existing bindings
	b := &Binder{Cfg: cfg, Host: host, Getenv: func(string) string { return "tok" }}
	wrote, skipped, _, failed, err := b.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 0 || skipped != 1 || failed != 0 {
		t.Fatalf("wrote=%d skipped=%d failed=%d", wrote, skipped, failed)
	}
	if len(host.saved) != 0 {
		t.Fatalf("unexpected save: %#v", host.saved)
	}
}

func TestSyncOnceIdempotentSecondPass(t *testing.T) {
	host := &mockHost{
		entries: []HostAuthEntry{{
			ID: "id1", AuthIndex: "idx3", Name: "xai-c.json", Provider: "xai", Email: "c@example.com",
		}},
		files: map[string]json.RawMessage{
			"idx3": json.RawMessage(`{"type":"xai","email":"c@example.com"}`),
		},
		names: map[string]string{"idx3": "xai-c.json"},
	}
	cfg := Defaults()
	cfg.OnlyIfEmpty = false
	cfg.OverwriteExisting = true
	b := &Binder{Cfg: cfg, Host: host, Getenv: func(string) string { return "tok" }}
	if _, _, _, _, err := b.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	wrote, skipped, unchanged, failed, err := b.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 0 || failed != 0 {
		t.Fatalf("second pass wrote=%d failed=%d skipped=%d unchanged=%d", wrote, failed, skipped, unchanged)
	}
	if unchanged != 1 && skipped != 1 {
		// with overwrite true and same URL -> unchanged
		t.Fatalf("expected unchanged/skip, got wrote=%d skipped=%d unchanged=%d", wrote, skipped, unchanged)
	}
}

// noopSaveHost pretends writes succeed while the stored file never changes,
// simulating an external writer that keeps reverting the value.
type noopSaveHost struct{ mockHost }

func (m *noopSaveHost) SaveAuth(name string, rawJSON json.RawMessage) error { return nil }

func TestSyncOnceWriteBackoffWhenExternalWriterReverts(t *testing.T) {
	host := &noopSaveHost{mockHost{
		entries: []HostAuthEntry{{
			ID: "id1", AuthIndex: "idx9", Name: "codex-c.json", Provider: "codex",
		}},
		files: map[string]json.RawMessage{
			// stored value never changes even though SaveAuth "succeeds"
			"idx9": json.RawMessage(`{"type":"codex","proxy_url":"socks5h://oldpool.old:tok@resin:2260"}`),
		},
		names: map[string]string{"idx9": "codex-c.json"},
	}}
	cfg := Defaults()
	cfg.RebindOnChange = true
	cfg.ProxyByType = map[string]string{"codex": "socks5h://pool-a.{account}:tok1@resin:2260"}
	b := &Binder{Cfg: cfg, Host: host, Getenv: func(string) string { return "tok" }}
	wrote, _, _, failed, err := b.SyncOnce()
	if err != nil || wrote != 1 || failed != 0 {
		t.Fatalf("first pass wrote=%d failed=%d err=%v", wrote, failed, err)
	}
	wrote, skipped, _, failed, err := b.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 0 || failed != 0 {
		t.Fatalf("second pass must back off: wrote=%d failed=%d skipped=%d", wrote, failed, skipped)
	}
	found := false
	for _, l := range host.logs {
		if strings.Contains(l, "reason=write_backoff") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected write_backoff skip, logs=%v", host.logs)
	}
}

func TestSameProxyCompareToleratesRawNonASCII(t *testing.T) {
	// an old binding stored with raw non-ASCII and the properly escaped
	// target describe the same logical URL and must not trigger a rewrite
	raw := "socks5h://gpt-proxy.nv重授权-x.json:tok@resin:2260"
	escaped := RenderProxyEntry("socks5h://gpt-proxy.{account}:tok@resin:2260", "nv重授权-x.json")
	if !sameProxyURL(raw, escaped) {
		t.Fatalf("raw form must compare equal to escaped form:\nraw=%q\nesc=%q", raw, escaped)
	}
}

func TestSyncOnceSkipsRuntimeOnlyAndDisabled(t *testing.T) {
	host := &mockHost{
		entries: []HostAuthEntry{
			{AuthIndex: "r1", Name: "rt.json", RuntimeOnly: true, Provider: "xai"},
			{AuthIndex: "d1", Name: "dis.json", Disabled: true, Provider: "xai"},
		},
		files: map[string]json.RawMessage{},
		names: map[string]string{},
	}
	b := &Binder{Cfg: Defaults(), Host: host, Getenv: func(string) string { return "tok" }}
	wrote, skipped, _, failed, err := b.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 0 || skipped != 2 || failed != 0 {
		t.Fatalf("wrote=%d skipped=%d failed=%d", wrote, skipped, failed)
	}
}

func TestSyncOnceRebindsOnChange(t *testing.T) {
	host := &mockHost{
		entries: []HostAuthEntry{{
			ID: "id1", AuthIndex: "idx4", Name: "codex-a.json", Provider: "codex", Email: "a@example.com",
		}},
		files: map[string]json.RawMessage{
			"idx4": json.RawMessage(`{"type":"codex","email":"a@example.com","proxy_url":"socks5h://old-pool.oldacc:tok@resin:2260"}`),
		},
		names: map[string]string{"idx4": "codex-a.json"},
	}
	cfg := Defaults()
	cfg.RebindOnChange = true // opt-in: rewrite existing bindings when target changes
	cfg.ProxyByType = map[string]string{"codex": "socks5h://pool-a.{account}:tok1@resin:2260"}
	b := &Binder{Cfg: cfg, Host: host, Getenv: func(string) string { return "tok" }}
	wrote, skipped, unchanged, failed, err := b.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 1 || skipped != 0 || unchanged != 0 || failed != 0 {
		t.Fatalf("rebind stats wrote=%d skipped=%d unchanged=%d failed=%d", wrote, skipped, unchanged, failed)
	}
	var meta map[string]any
	if err := json.Unmarshal(host.saved["codex-a.json"], &meta); err != nil {
		t.Fatal(err)
	}
	proxy, _ := meta["proxy_url"].(string)
	// {account} expanded with the account derived from auth_id (id1)
	if proxy != "socks5h://pool-a.id1:tok1@resin:2260" {
		t.Fatalf("proxy = %q", proxy)
	}
}

func TestSyncOnceRebindSkipsWhenDisabled(t *testing.T) {
	host := &mockHost{
		entries: []HostAuthEntry{{
			ID: "id1", AuthIndex: "idx5", Name: "codex-b.json", Provider: "codex",
		}},
		files: map[string]json.RawMessage{
			"idx5": json.RawMessage(`{"type":"codex","proxy_url":"socks5h://old-pool.oldacc:tok@resin:2260"}`),
		},
		names: map[string]string{"idx5": "codex-b.json"},
	}
	cfg := Defaults()
	cfg.RebindOnChange = false
	cfg.ProxyByType = map[string]string{"codex": "socks5h://pool-a.{account}:tok1@resin:2260"}
	b := &Binder{Cfg: cfg, Host: host, Getenv: func(string) string { return "tok" }}
	wrote, skipped, _, failed, err := b.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 0 || skipped != 1 || failed != 0 {
		t.Fatalf("wrote=%d skipped=%d failed=%d", wrote, skipped, failed)
	}
}

func TestSyncOnceOtherFallbackForUnknownType(t *testing.T) {
	host := &mockHost{
		entries: []HostAuthEntry{{
			ID: "id9", AuthIndex: "idx6", Name: "gemini-a.json", Provider: "gemini", Email: "g@example.com",
		}},
		files: map[string]json.RawMessage{
			"idx6": json.RawMessage(`{"type":"gemini","email":"g@example.com"}`),
		},
		names: map[string]string{"idx6": "gemini-a.json"},
	}
	cfg := Defaults()
	cfg.OtherProxyURL = "socks5h://pool-b.{account}:tok2@resin:2260"
	b := &Binder{Cfg: cfg, Host: host, Getenv: func(string) string { return "tok" }}
	wrote, _, _, failed, err := b.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 1 || failed != 0 {
		t.Fatalf("wrote=%d failed=%d", wrote, failed)
	}
	var meta map[string]any
	if err := json.Unmarshal(host.saved["gemini-a.json"], &meta); err != nil {
		t.Fatal(err)
	}
	proxy, _ := meta["proxy_url"].(string)
	if proxy != "socks5h://pool-b.id9:tok2@resin:2260" {
		t.Fatalf("proxy = %q", proxy)
	}
}

func TestSyncOnceVerbatimEntrySharedByType(t *testing.T) {
	host := &mockHost{
		entries: []HostAuthEntry{{
			ID: "id2", AuthIndex: "idx7", Name: "claude-a.json", Provider: "claude", Email: "c@example.com",
		}},
		files: map[string]json.RawMessage{
			"idx7": json.RawMessage(`{"type":"claude","email":"c@example.com"}`),
		},
		names: map[string]string{"idx7": "claude-a.json"},
	}
	cfg := Defaults()
	// no {account} placeholder: the entry is used verbatim, shared by the type
	cfg.ProxyByType = map[string]string{"claude": "http://alice:s3cret@proxy.example:8080"}
	b := &Binder{Cfg: cfg, Host: host, Getenv: func(string) string { return "tok" }}
	wrote, _, _, failed, err := b.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 1 || failed != 0 {
		t.Fatalf("wrote=%d failed=%d", wrote, failed)
	}
	var meta map[string]any
	if err := json.Unmarshal(host.saved["claude-a.json"], &meta); err != nil {
		t.Fatal(err)
	}
	proxy, _ := meta["proxy_url"].(string)
	if proxy != "http://alice:s3cret@proxy.example:8080" {
		t.Fatalf("plain proxy should be written verbatim: %q", proxy)
	}
}

func TestSyncOnceDottedIdentitySharedAcrossType(t *testing.T) {
	host := &mockHost{
		entries: []HostAuthEntry{
			{ID: "id1", AuthIndex: "i1", Name: "kimi-a.json", Provider: "kimi"},
			{ID: "id2", AuthIndex: "i2", Name: "kimi-b.json", Provider: "kimi"},
		},
		files: map[string]json.RawMessage{
			"i1": json.RawMessage(`{"type":"kimi"}`),
			"i2": json.RawMessage(`{"type":"kimi"}`),
		},
		names: map[string]string{"i1": "kimi-a.json", "i2": "kimi-b.json"},
	}
	cfg := Defaults()
	cfg.ProxyByType = map[string]string{"kimi": "socks5h://pool-x.fixedacc:tok@resin:2260"}
	b := &Binder{Cfg: cfg, Host: host, Getenv: func(string) string { return "tok" }}
	wrote, _, _, failed, err := b.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 2 || failed != 0 {
		t.Fatalf("wrote=%d failed=%d", wrote, failed)
	}
	want := "socks5h://pool-x.fixedacc:tok@resin:2260"
	for name, raw := range host.saved {
		var meta map[string]any
		if err := json.Unmarshal(raw, &meta); err != nil {
			t.Fatal(err)
		}
		if proxy, _ := meta["proxy_url"].(string); proxy != want {
			t.Fatalf("%s proxy = %q, want fixed identity %q", name, proxy, want)
		}
	}
}

func TestSyncOnceLegacyPathStillWorksWithoutTypedConfig(t *testing.T) {
	host := &mockHost{
		entries: []HostAuthEntry{{
			ID: "id1", AuthIndex: "idx8", Name: "xai-d.json", Provider: "xai",
		}},
		files: map[string]json.RawMessage{"idx8": json.RawMessage(`{"type":"xai"}`)},
		names: map[string]string{"idx8": "xai-d.json"},
	}
	cfg := Defaults()
	b := &Binder{Cfg: cfg, Host: host, Getenv: func(string) string { return "tok" }}
	wrote, _, _, failed, err := b.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if wrote != 1 || failed != 0 {
		t.Fatalf("wrote=%d failed=%d", wrote, failed)
	}
	var meta map[string]any
	if err := json.Unmarshal(host.saved["xai-d.json"], &meta); err != nil {
		t.Fatal(err)
	}
	proxy, _ := meta["proxy_url"].(string)
	if !contains(proxy, "default.id1") {
		t.Fatalf("legacy platform expansion broken: %q", proxy)
	}
}
