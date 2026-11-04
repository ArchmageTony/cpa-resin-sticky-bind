package stickybind

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// HostAuthEntry is one credential from host.auth.list.
type HostAuthEntry struct {
	ID          string `json:"id,omitempty"`
	AuthIndex   string `json:"auth_index,omitempty"`
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Label       string `json:"label,omitempty"`
	Status      string `json:"status,omitempty"`
	Disabled    bool   `json:"disabled,omitempty"`
	Unavailable bool   `json:"unavailable,omitempty"`
	RuntimeOnly bool   `json:"runtime_only,omitempty"`
	Source      string `json:"source,omitempty"`
	Path        string `json:"path,omitempty"`
	Email       string `json:"email,omitempty"`
}

// AuthGetResult is host.auth.get payload.
type AuthGetResult struct {
	AuthIndex string          `json:"auth_index"`
	Name      string          `json:"name,omitempty"`
	Path      string          `json:"path,omitempty"`
	JSON      json.RawMessage `json:"json"`
}

// Action describes one sync decision.
type Action string

const (
	ActionSkip      Action = "skip"
	ActionWrite     Action = "write"
	ActionUnchanged Action = "unchanged"
)

// Decision is the result of evaluating one credential.
type Decision struct {
	Action    Action
	Reason    string
	AuthIndex string
	Name      string
	Provider  string
	Platform  string
	Account   string
	ProxyURL  string
	SaveName  string
	SaveJSON  json.RawMessage
	Redacted  string
}

// HostAPI is the minimal host surface used by the binder.
type HostAPI interface {
	ListAuths() ([]HostAuthEntry, error)
	GetAuth(authIndex string) (AuthGetResult, error)
	SaveAuth(name string, rawJSON json.RawMessage) error
	Log(level, message string)
}

// Binder performs credential proxy binding.
type Binder struct {
	Cfg    Config
	Host   HostAPI
	Getenv func(string) string

	// typedCache caches NormalizeProxyEntry results per raw entry for one
	// sync pass. Single-goroutine use only (runtime loop / manual sync).
	typedCache map[string]typedEntry

	// lastWritten remembers the last proxy URL the binder wrote per auth
	// index (across sync passes of this Binder). If a fresh pass computes
	// the same target while the stored value still differs, something
	// external keeps reverting the file; rewriting forever would only churn
	// the watcher, so the binder backs off instead.
	lastWritten map[string]string

	// backoffSkips counts consecutive write_backoff skips per auth index.
	// The backoff is bounded: after MaxBackoffSkips skipped passes the binder
	// retries the write once, so a transient external writer (e.g. a stale
	// plugin instance that has since stopped) cannot pin a wrong binding
	// forever.
	backoffSkips map[string]int
}

// MaxBackoffSkips bounds how many consecutive passes one credential stays in
// write_backoff before the binder retries the write.
const MaxBackoffSkips = 10

// ResetState drops cross-pass memory (write history and backoff counters).
// Call it whenever the effective configuration changes so a new target is
// never mistaken for an external revert of an old one.
func (b *Binder) ResetState() {
	if b == nil {
		return
	}
	b.lastWritten = nil
	b.backoffSkips = nil
}

// SyncOnce enumerates credentials and binds empty proxy_url fields.
func (b *Binder) SyncOnce() (wrote, skipped, unchanged, failed int, err error) {
	if b == nil || b.Host == nil {
		return 0, 0, 0, 0, fmt.Errorf("binder host is nil")
	}
	cfg := b.Cfg.Normalize()
	if !cfg.Enabled {
		b.Host.Log("info", "sticky-bind disabled; skip sync")
		return 0, 0, 0, 0, nil
	}
	// One line per pass so the effective runtime configuration is visible in
	// the field (page saves must show up here on the next pass).
	b.Host.Log("info", fmt.Sprintf("sticky-bind config: strategy=%s rebind=%v typed_rows=%d other_set=%v", cfg.AccountStrategy, cfg.RebindOnChange, len(cfg.ProxyByType), cfg.OtherProxyURL != ""))
	b.typedCache = map[string]typedEntry{}
	// Legacy resin_proxy_url is optional once proxy_by_type is configured:
	// a missing token here only disables the fallback path, not the sync.
	legacyProxy, legacyErr := ParseResinProxyURL(cfg.ResinProxyURL, cfg.ProxyTokenEnv, b.Getenv)
	if legacyErr != nil {
		legacyProxy = ParsedProxy{}
		b.Host.Log("info", "legacy resin_proxy_url unavailable: "+legacyErr.Error())
	}

	entries, errList := b.Host.ListAuths()
	if errList != nil {
		return 0, 0, 0, 0, errList
	}
	for _, entry := range entries {
		dec, errDecide := b.decide(cfg, legacyProxy, legacyErr, entry)
		if errDecide != nil {
			failed++
			b.Host.Log("error", fmt.Sprintf("decide failed name=%s auth_index=%s err=%v", entry.Name, entry.AuthIndex, errDecide))
			continue
		}
		switch dec.Action {
		case ActionSkip:
			skipped++
			b.Host.Log("info", fmt.Sprintf("skip name=%s auth_index=%s reason=%s", dec.Name, dec.AuthIndex, dec.Reason))
		case ActionUnchanged:
			unchanged++
			b.Host.Log("info", fmt.Sprintf("unchanged name=%s auth_index=%s platform=%s account=%s proxy=%s", dec.Name, dec.AuthIndex, dec.Platform, dec.Account, dec.Redacted))
		case ActionWrite:
			if errSave := b.Host.SaveAuth(dec.SaveName, dec.SaveJSON); errSave != nil {
				failed++
				b.Host.Log("error", fmt.Sprintf("save failed name=%s auth_index=%s err=%v", dec.Name, dec.AuthIndex, errSave))
				continue
			}
			if b.lastWritten == nil {
				b.lastWritten = map[string]string{}
			}
			b.lastWritten[dec.AuthIndex] = dec.ProxyURL
			delete(b.backoffSkips, dec.AuthIndex)
			wrote++
			b.Host.Log("info", fmt.Sprintf("bound name=%s auth_index=%s platform=%s account=%s proxy=%s", dec.Name, dec.AuthIndex, dec.Platform, dec.Account, dec.Redacted))
		}
	}
	return wrote, skipped, unchanged, failed, nil
}

func (b *Binder) decide(cfg Config, legacyProxy ParsedProxy, legacyErr error, entry HostAuthEntry) (Decision, error) {
	dec := Decision{
		AuthIndex: strings.TrimSpace(entry.AuthIndex),
		Name:      strings.TrimSpace(entry.Name),
		Provider:  firstNonEmpty(entry.Provider, entry.Type),
	}
	if dec.AuthIndex == "" {
		return Decision{Action: ActionSkip, Reason: "missing_auth_index", Name: dec.Name}, nil
	}
	if entry.RuntimeOnly {
		return Decision{Action: ActionSkip, Reason: "runtime_only", AuthIndex: dec.AuthIndex, Name: dec.Name, Provider: dec.Provider}, nil
	}
	if entry.Disabled || strings.EqualFold(entry.Status, "disabled") {
		return Decision{Action: ActionSkip, Reason: "disabled", AuthIndex: dec.AuthIndex, Name: dec.Name, Provider: dec.Provider}, nil
	}
	if !ProviderAllowed(cfg, dec.Provider) {
		return Decision{Action: ActionSkip, Reason: "provider_filtered", AuthIndex: dec.AuthIndex, Name: dec.Name, Provider: dec.Provider}, nil
	}

	got, errGet := b.Host.GetAuth(dec.AuthIndex)
	if errGet != nil {
		return Decision{}, errGet
	}
	saveName := firstNonEmpty(strings.TrimSpace(got.Name), dec.Name)
	if saveName == "" {
		return Decision{Action: ActionSkip, Reason: "missing_filename", AuthIndex: dec.AuthIndex, Name: dec.Name, Provider: dec.Provider}, nil
	}
	if !strings.HasSuffix(strings.ToLower(saveName), ".json") {
		saveName = saveName + ".json"
	}
	saveName = filepath.Base(saveName)

	meta, existingProxy, errMeta := parseAuthJSON(got.JSON)
	if errMeta != nil {
		return Decision{}, errMeta
	}
	// v0.1 behaviour: leave existing bindings alone unless rebind is on or
	// the operator explicitly allows overwriting.
	if existingProxy != "" && !cfg.RebindOnChange && cfg.OnlyIfEmpty && !cfg.OverwriteExisting {
		platform, account, _ := ExtractAccountFromProxyURL(existingProxy)
		return Decision{
			Action:    ActionSkip,
			Reason:    "proxy_url_present",
			AuthIndex: dec.AuthIndex,
			Name:      saveName,
			Provider:  dec.Provider,
			Platform:  platform,
			Account:   account,
			ProxyURL:  existingProxy,
			Redacted:  RedactProxyURL(existingProxy),
		}, nil
	}

	authID := firstNonEmpty(entry.ID, metaString(meta, "id"), saveName)
	email := firstNonEmpty(entry.Email, metaString(meta, "email"))
	sub := metaString(meta, "sub")
	accountIn := AccountInput{
		AuthID:   authID,
		Email:    email,
		Sub:      sub,
		Filename: saveName,
		Provider: dec.Provider,
	}

	proxyURL, platform, account, errBuild := b.buildTargetProxy(cfg, legacyProxy, legacyErr, dec.Provider, accountIn)
	if errBuild != nil {
		return Decision{}, errBuild
	}
	if proxyURL == "" {
		return Decision{Action: ActionSkip, Reason: "proxy_unavailable", AuthIndex: dec.AuthIndex, Name: saveName, Provider: dec.Provider}, nil
	}
	if existingProxy != "" && sameProxyURL(existingProxy, proxyURL) {
		return Decision{
			Action:    ActionUnchanged,
			Reason:    "already_bound",
			AuthIndex: dec.AuthIndex,
			Name:      saveName,
			Provider:  dec.Provider,
			Platform:  platform,
			Account:   account,
			ProxyURL:  existingProxy,
			Redacted:  RedactProxyURL(existingProxy),
		}, nil
	}
	if existingProxy != "" && !cfg.RebindOnChange && !cfg.OverwriteExisting {
		// rebind disabled and no explicit overwrite permission
		platform, account, _ := ExtractAccountFromProxyURL(existingProxy)
		return Decision{
			Action:    ActionSkip,
			Reason:    "proxy_url_present",
			AuthIndex: dec.AuthIndex,
			Name:      saveName,
			Provider:  dec.Provider,
			Platform:  platform,
			Account:   account,
			ProxyURL:  existingProxy,
			Redacted:  RedactProxyURL(existingProxy),
		}, nil
	}
	if existingProxy != "" && !sameProxyURL(existingProxy, proxyURL) {
		// convergence guard: the identical target was written before and the
		// stored value still differs, so an external writer keeps reverting
		// the file. Back off instead of churning every sync cycle.
		if last, ok := b.lastWritten[dec.AuthIndex]; ok && last == proxyURL {
			if b.backoffSkips == nil {
				b.backoffSkips = map[string]int{}
			}
			b.backoffSkips[dec.AuthIndex]++
			n := b.backoffSkips[dec.AuthIndex]
			if n > MaxBackoffSkips {
				// bounded backoff: retry the write once, then start counting again
				b.Host.Log("warn", fmt.Sprintf("write_backoff expired, retrying name=%s auth_index=%s observed=%s target=%s", saveName, dec.AuthIndex, RedactProxyURL(existingProxy), RedactProxyURL(proxyURL)))
				delete(b.backoffSkips, dec.AuthIndex)
				goto write
			}
			if n == 1 {
				b.Host.Log("warn", fmt.Sprintf("external writer reverted binding; backing off name=%s auth_index=%s observed=%s target=%s retry_after_passes=%d", saveName, dec.AuthIndex, RedactProxyURL(existingProxy), RedactProxyURL(proxyURL), MaxBackoffSkips))
			}
			return Decision{
				Action:    ActionSkip,
				Reason:    "write_backoff",
				AuthIndex: dec.AuthIndex,
				Name:      saveName,
				Provider:  dec.Provider,
				Platform:  platform,
				Account:   account,
				ProxyURL:  existingProxy,
				Redacted:  RedactProxyURL(existingProxy),
			}, nil
		}
	}

write:
	reason := "bind"
	if existingProxy != "" {
		reason = "rebind"
	}
	meta["proxy_url"] = proxyURL
	raw, errMarshal := json.Marshal(meta)
	if errMarshal != nil {
		return Decision{}, errMarshal
	}
	return Decision{
		Action:    ActionWrite,
		Reason:    reason,
		AuthIndex: dec.AuthIndex,
		Name:      saveName,
		Provider:  dec.Provider,
		Platform:  platform,
		Account:   account,
		ProxyURL:  proxyURL,
		SaveName:  saveName,
		SaveJSON:  raw,
		Redacted:  RedactProxyURL(proxyURL),
	}, nil
}

// typedEntry is one normalized proxy_by_type / other_proxy_url entry.
type typedEntry struct {
	normalized   string
	accountBound bool // entry contains the {account} placeholder (sticky)
}

// buildTargetProxy resolves the proxy URL for one credential:
// proxy_by_type[type] first, then other_proxy_url, then the legacy
// resin_proxy_url + platform chain. Entries containing {account} are
// account-bound: the placeholder is replaced with the per-credential
// account; entries without it are used verbatim and shared by the type.
// platform/account are display values.
func (b *Binder) buildTargetProxy(cfg Config, legacyProxy ParsedProxy, legacyErr error, provider string, accountIn AccountInput) (proxyURL, platform, account string, err error) {
	if input := cfg.ResolveProxyInput(provider); input != "" {
		entry, errNorm := b.normalizeTypedCached(input)
		if errNorm != nil {
			return "", "", "", errNorm
		}
		proxyURL = entry.normalized
		if entry.accountBound {
			account = BuildAccount(cfg, accountIn)
			if strings.TrimSpace(account) == "" {
				return "", "", "", fmt.Errorf("account is empty")
			}
			proxyURL = RenderProxyEntry(entry.normalized, account)
		}
		platform, account = identityDisplay(proxyURL)
		return proxyURL, platform, account, nil
	}
	if legacyErr != nil {
		// no usable proxy source for this credential; caller records a skip
		return "", "", "", nil
	}
	platform = ResolvePlatform(cfg, accountIn.AuthID, provider)
	if platform == "" {
		return "", "", "", nil
	}
	account = BuildAccount(cfg, accountIn)
	proxyURL, errBuild := BuildProxyURL(legacyProxy, platform, account)
	if errBuild != nil {
		return "", "", "", errBuild
	}
	return proxyURL, platform, account, nil
}

// identityDisplay maps a rendered proxy URL to Platform/Account display
// fields when the username follows the Platform.Account shape.
func identityDisplay(proxyURL string) (platform, account string) {
	platform, account, ok := ExtractAccountFromProxyURL(proxyURL)
	if !ok {
		return "", ""
	}
	return platform, account
}

func (b *Binder) normalizeTypedCached(raw string) (typedEntry, error) {
	if cached, ok := b.typedCache[raw]; ok {
		return cached, nil
	}
	normalized, accountBound, err := NormalizeProxyEntry(raw)
	if err != nil {
		return typedEntry{}, err
	}
	entry := typedEntry{normalized: normalized, accountBound: accountBound}
	if b.typedCache == nil {
		b.typedCache = map[string]typedEntry{}
	}
	b.typedCache[raw] = entry
	return entry, nil
}

func parseAuthJSON(raw json.RawMessage) (map[string]any, string, error) {
	if len(bytesTrimSpace(raw)) == 0 {
		return nil, "", fmt.Errorf("auth json is empty")
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, "", fmt.Errorf("invalid auth json: %w", err)
	}
	if meta == nil {
		meta = map[string]any{}
	}
	proxy := strings.TrimSpace(metaString(meta, "proxy_url"))
	return meta, proxy, nil
}

func metaString(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	if v, ok := meta[key]; ok {
		return StringFromAny(v)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func bytesTrimSpace(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}

func sameProxyURL(a, b string) bool {
	return normalizeProxyCompare(a) == normalizeProxyCompare(b)
}

// normalizeProxyCompare canonicalizes a proxy URL for equality checks. URLs
// the parser rejects (e.g. raw non-ASCII in the userinfo, as written by
// older plugin versions) are compared on their percent-encoded form, so a
// stored raw form and its properly escaped target are recognised as equal
// instead of triggering a rewrite every sync cycle.
func normalizeProxyCompare(raw string) string {
	raw = strings.TrimSpace(raw)
	if u, err := url.Parse(raw); err == nil {
		return u.String()
	}
	return escapeNonASCII(raw)
}

func parseLooseURL(raw string) (*url.URL, error) {
	return url.Parse(strings.TrimSpace(raw))
}
