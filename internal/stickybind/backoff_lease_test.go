package stickybind

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func newRevertingBinder() (*Binder, *noopSaveHost) {
	host := &noopSaveHost{mockHost{
		entries: []HostAuthEntry{{ID: "id1", AuthIndex: "idx9", Name: "codex-c.json", Provider: "codex"}},
		files: map[string]json.RawMessage{
			"idx9": json.RawMessage(`{"type":"codex","proxy_url":"socks5h://oldpool.old:tok@resin:2260"}`),
		},
		names: map[string]string{"idx9": "codex-c.json"},
	}}
	cfg := Defaults()
	cfg.RebindOnChange = true
	cfg.ProxyByType = map[string]string{"codex": "socks5h://pool-a.{account}:tok1@resin:2260"}
	return &Binder{Cfg: cfg, Host: host, Getenv: func(string) string { return "tok" }}, host
}

func TestWriteBackoffIsBounded(t *testing.T) {
	b, host := newRevertingBinder()
	if w, _, _, _, _ := b.SyncOnce(); w != 1 {
		t.Fatalf("first pass must write, got %d", w)
	}
	for i := 0; i < MaxBackoffSkips; i++ {
		if w, _, _, _, _ := b.SyncOnce(); w != 0 {
			t.Fatalf("pass %d must back off", i)
		}
	}
	if w, _, _, _, _ := b.SyncOnce(); w != 1 {
		t.Fatalf("backoff must expire after %d passes and retry", MaxBackoffSkips)
	}
	var warned, leaked bool
	for _, l := range host.logs {
		if strings.Contains(l, "external writer reverted binding") && strings.Contains(l, "observed=") && strings.Contains(l, "target=") {
			warned = true
		}
		if strings.Contains(l, "tok1") || strings.Contains(l, ":tok@") {
			leaked = true
		}
	}
	if !warned || leaked {
		t.Fatalf("warned=%v leaked=%v logs=%v", warned, leaked, host.logs)
	}
}

func TestResetStateClearsBackoff(t *testing.T) {
	b, _ := newRevertingBinder()
	b.SyncOnce()
	if w, _, _, _, _ := b.SyncOnce(); w != 0 {
		t.Fatal("expected backoff")
	}
	b.ResetState()
	if w, _, _, _, _ := b.SyncOnce(); w != 1 {
		t.Fatal("after ResetState the binder must write again")
	}
}

func TestLeaseNewestClaimWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "instance.lease")
	a := Lease{Path: path, ID: NewInstanceID()}
	b := Lease{Path: path, ID: NewInstanceID()}
	if a.ID == b.ID {
		t.Fatal("instance ids must differ")
	}
	if held, _ := a.Held(); !held {
		t.Fatal("missing lease must fail open")
	}
	if err := a.Claim(); err != nil {
		t.Fatal(err)
	}
	if held, _ := a.Held(); !held {
		t.Fatal("a should hold after claim")
	}
	if err := b.Claim(); err != nil {
		t.Fatal(err)
	}
	if held, owner := a.Held(); held || owner != b.ID {
		t.Fatalf("a must be superseded by b, held=%v owner=%s", held, owner)
	}
	if held, _ := b.Held(); !held {
		t.Fatal("b must hold")
	}
}

func TestLeaseFileDefaultAndParse(t *testing.T) {
	if Defaults().Normalize().LeaseFile != DefaultLeaseFile {
		t.Fatal("default lease file")
	}
	c := Config{}.Normalize()
	if c.LeaseFile != DefaultLeaseFile {
		t.Fatal("normalize must fill lease file")
	}
}
