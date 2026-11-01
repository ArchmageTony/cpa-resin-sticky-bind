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
	ActionSkip     Action = "skip"
	ActionWrite    Action = "write"
	ActionUnchanged Action = "unchanged"
)

// Decision is the result of evaluating one credential.
type Decision struct {
	Action     Action
	Reason     string
	AuthIndex  string
	Name       string
	Provider   string
	Platform   string
	Account    string
	ProxyURL   string
	SaveName   string
	SaveJSON   json.RawMessage
	Redacted   string
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

func normalizeProxyCompare(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	// compare full string with password included; equality is exact after trim
	return u.String()
}

func parseLooseURL(raw string) (*url.URL, error) {
	return url.Parse(strings.TrimSpace(raw))
}
