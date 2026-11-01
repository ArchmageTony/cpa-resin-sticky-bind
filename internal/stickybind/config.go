package stickybind

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	DefaultProxyTokenEnv      = "RESIN_PROXY_TOKEN"
	DefaultPlatform           = "default"
	DefaultAccountStrategy    = "auth_id"
	DefaultSyncIntervalSec    = 30
	DefaultResinProxyURL      = "socks5h://resin:2260"
	MaxAccountLen             = 64
	AccountStrategyAuthID     = "auth_id"
	AccountStrategyEmail      = "email"
	AccountStrategySub        = "sub"
	AccountStrategyFilename   = "filename"
)

// KnownProviders is the built-in list of credential types CPA supports.
// The management page unions this with types seen at runtime via host.auth.list.
var KnownProviders = []string{
	"codex",
	"claude",
	"gemini",
	"antigravity",
	"qwen",
	"iflow",
	"kimi",
	"xai",
	"aistudio",
	"vertex",
	"devin",
}

// OtherProviderKey is the pseudo type used as fallback for any credential
// type without a dedicated proxy_by_type entry.
const OtherProviderKey = "other"

// Config is the runtime plugin configuration.
type Config struct {
	Enabled             bool              `json:"enabled" yaml:"enabled"`
	ResinProxyURL       string            `json:"resin_proxy_url" yaml:"resin_proxy_url"`
	ProxyTokenEnv       string            `json:"proxy_token_env" yaml:"proxy_token_env"`
	DefaultPlatform     string            `json:"default_platform" yaml:"default_platform"`
	PlatformByProvider  map[string]string `json:"platform_by_provider" yaml:"platform_by_provider"`
	PlatformByAuthID    map[string]string `json:"platform_by_auth_id" yaml:"platform_by_auth_id"`
	AccountStrategy     string            `json:"account_strategy" yaml:"account_strategy"`
	AccountPrefix       string            `json:"account_prefix" yaml:"account_prefix"`
	SyncIntervalSeconds int               `json:"sync_interval_seconds" yaml:"sync_interval_seconds"`
	OnlyIfEmpty         bool              `json:"only_if_empty" yaml:"only_if_empty"`
	OverwriteExisting   bool              `json:"overwrite_existing" yaml:"overwrite_existing"`
	IncludeProviders    []string          `json:"include_providers" yaml:"include_providers"`
	ExcludeProviders    []string          `json:"exclude_providers" yaml:"exclude_providers"`
	ProxyByType         map[string]string `json:"proxy_by_type" yaml:"proxy_by_type"`
	StickyByType        map[string]string `json:"sticky_by_type" yaml:"sticky_by_type"`
	OtherProxyURL       string            `json:"other_proxy_url" yaml:"other_proxy_url"`
	ExpandStickyAccount bool              `json:"expand_sticky_account" yaml:"expand_sticky_account"`
	RebindOnChange      bool              `json:"rebind_on_change" yaml:"rebind_on_change"`
}

// Defaults returns the first-release defaults.
func Defaults() Config {
	return Config{
		Enabled:             true,
		ResinProxyURL:       DefaultResinProxyURL,
		ProxyTokenEnv:       DefaultProxyTokenEnv,
		DefaultPlatform:     DefaultPlatform,
		PlatformByProvider:  map[string]string{},
		PlatformByAuthID:    map[string]string{},
		AccountStrategy:     DefaultAccountStrategy,
		AccountPrefix:       "",
		SyncIntervalSeconds: DefaultSyncIntervalSec,
		OnlyIfEmpty:         true,
		OverwriteExisting:   false,
		IncludeProviders:    nil,
		ExcludeProviders:    nil,
		ProxyByType:         map[string]string{},
		StickyByType:        map[string]string{},
		OtherProxyURL:       "",
		ExpandStickyAccount: true,
		RebindOnChange:      false,
	}
}

// Normalize fills defaults and cleans fields.
func (c Config) Normalize() Config {
	out := c
	if strings.TrimSpace(out.ResinProxyURL) == "" {
		out.ResinProxyURL = DefaultResinProxyURL
	}
	if strings.TrimSpace(out.ProxyTokenEnv) == "" {
		out.ProxyTokenEnv = DefaultProxyTokenEnv
	}
	if strings.TrimSpace(out.DefaultPlatform) == "" {
		out.DefaultPlatform = DefaultPlatform
	}
	if strings.TrimSpace(out.AccountStrategy) == "" {
		out.AccountStrategy = DefaultAccountStrategy
	}
	if out.SyncIntervalSeconds <= 0 {
		out.SyncIntervalSeconds = DefaultSyncIntervalSec
	}
	if out.PlatformByProvider == nil {
		out.PlatformByProvider = map[string]string{}
	}
	if out.PlatformByAuthID == nil {
		out.PlatformByAuthID = map[string]string{}
	}
	out.ResinProxyURL = strings.TrimSpace(out.ResinProxyURL)
	out.ProxyTokenEnv = strings.TrimSpace(out.ProxyTokenEnv)
	out.DefaultPlatform = strings.TrimSpace(out.DefaultPlatform)
	out.AccountStrategy = strings.ToLower(strings.TrimSpace(out.AccountStrategy))
	out.AccountPrefix = strings.TrimSpace(out.AccountPrefix)
	out.IncludeProviders = cleanStringList(out.IncludeProviders)
	out.ExcludeProviders = cleanStringList(out.ExcludeProviders)
	out.PlatformByProvider = cleanStringMap(out.PlatformByProvider)
	out.PlatformByAuthID = cleanStringMap(out.PlatformByAuthID)
	out.ProxyByType = cleanStringMap(out.ProxyByType)
	out.StickyByType = cleanStringMap(out.StickyByType)
	out.OtherProxyURL = strings.TrimSpace(out.OtherProxyURL)
	return out
}

// StickyForType reports whether a per-type proxy entry should expand its
// dotless sticky identity with the per-credential account. Unlisted types
// default to true because Resin sticky URLs are this plugin's primary form.
// The "other" fallback row uses OtherProviderKey as its key.
func (c Config) StickyForType(provider string) bool {
	if len(c.StickyByType) > 0 {
		key := strings.TrimSpace(provider)
		if key != "" {
			if v, ok := c.StickyByType[key]; ok {
				return BoolFromAny(v, true)
			}
			lower := strings.ToLower(key)
			for k, v := range c.StickyByType {
				if strings.ToLower(strings.TrimSpace(k)) == lower {
					return BoolFromAny(v, true)
				}
			}
		}
	}
	return c.ExpandStickyAccount
}

// ParsedProxy is the resolved resin proxy endpoint without account identity.
type ParsedProxy struct {
	Scheme   string
	Host     string
	Port     string
	Username string // sticky identity from the URL username, e.g. Resin Platform
	Password string // resolved token
	RawURL   string
}

// ParseResinProxyURL parses resin_proxy_url and resolves password.
// The URL username is ignored (treated as token when no password) so the
// legacy path always rebuilds identity as Platform.Account itself.
func ParseResinProxyURL(raw string, tokenEnv string, getenv func(string) string) (ParsedProxy, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ParsedProxy{}, fmt.Errorf("resin_proxy_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ParsedProxy{}, fmt.Errorf("invalid resin_proxy_url: %w", err)
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme == "" {
		return ParsedProxy{}, fmt.Errorf("resin_proxy_url scheme is required")
	}
	switch scheme {
	case "socks5", "socks5h", "http", "https":
	default:
		return ParsedProxy{}, fmt.Errorf("unsupported resin_proxy_url scheme %q", scheme)
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return ParsedProxy{}, fmt.Errorf("resin_proxy_url host is required")
	}
	port := strings.TrimSpace(u.Port())
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			port = "1080"
		}
	}
	password := ""
	if u.User != nil {
		if p, ok := u.User.Password(); ok {
			password = p
		}
		// Accept socks5h://<token>@host:port (token placed in username, no password).
		if strings.TrimSpace(password) == "" {
			password = strings.TrimSpace(u.User.Username())
		}
	}
	if strings.TrimSpace(password) == "" {
		envName := strings.TrimSpace(tokenEnv)
		if envName == "" {
			envName = DefaultProxyTokenEnv
		}
		if getenv == nil {
			getenv = os.Getenv
		}
		password = strings.TrimSpace(getenv(envName))
	}
	if password == "" {
		return ParsedProxy{}, fmt.Errorf("proxy token missing: set password in resin_proxy_url or env %s", strings.TrimSpace(tokenEnv))
	}
	return ParsedProxy{
		Scheme:   scheme,
		Host:     host,
		Port:     port,
		Password: password,
		RawURL:   raw,
	}, nil
}

// ParseTypedProxyURL parses one proxy_by_type / other_proxy_url entry and
// resolves its token. Unlike ParseResinProxyURL the URL username is preserved
// as the sticky identity: "socks5h://US:<token>@host" keeps Username "US",
// while the username-only form "socks5h://<token>@host" still means token.
func ParseTypedProxyURL(raw string, tokenEnv string, getenv func(string) string) (ParsedProxy, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ParsedProxy{}, fmt.Errorf("proxy url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ParsedProxy{}, fmt.Errorf("invalid proxy url: %w", err)
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme == "" {
		return ParsedProxy{}, fmt.Errorf("proxy url scheme is required")
	}
	switch scheme {
	case "socks5", "socks5h", "http", "https":
	default:
		return ParsedProxy{}, fmt.Errorf("unsupported proxy url scheme %q", scheme)
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return ParsedProxy{}, fmt.Errorf("proxy url host is required")
	}
	port := strings.TrimSpace(u.Port())
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		default:
			port = "1080"
		}
	}
	username := ""
	password := ""
	if u.User != nil {
		username = strings.TrimSpace(u.User.Username())
		if p, ok := u.User.Password(); ok {
			password = p
		}
	}
	if strings.TrimSpace(password) == "" {
		// username-only form: the username carries the token, no identity.
		password = username
		username = ""
	}
	if strings.TrimSpace(password) == "" {
		envName := strings.TrimSpace(tokenEnv)
		if envName == "" {
			envName = DefaultProxyTokenEnv
		}
		if getenv == nil {
			getenv = os.Getenv
		}
		password = strings.TrimSpace(getenv(envName))
	}
	if password == "" {
		return ParsedProxy{}, fmt.Errorf("proxy token missing: set password in url or env %s", strings.TrimSpace(tokenEnv))
	}
	return ParsedProxy{
		Scheme:   scheme,
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		RawURL:   raw,
	}, nil
}

// BuildTypedProxyURL builds the final proxy URL for one credential from a
// per-type proxy entry. When the entry has a dotless sticky identity (e.g. a
// Resin platform) and expand is true, account is appended as identity.account;
// identities that already contain a dot are kept as-is for every credential.
func BuildTypedProxyURL(proxy ParsedProxy, account string, expand bool) (string, error) {
	if proxy.Password == "" {
		return "", fmt.Errorf("proxy token missing")
	}
	var userinfo *url.Userinfo
	switch {
	case proxy.Username != "":
		identity := proxy.Username
		if expand && !strings.Contains(identity, ".") {
			account = strings.TrimSpace(account)
			if account == "" {
				return "", fmt.Errorf("sticky identity %q requires an account", identity)
			}
			identity = identity + "." + account
		}
		userinfo = url.UserPassword(identity, proxy.Password)
	default:
		userinfo = url.User(proxy.Password)
	}
	out := &url.URL{
		Scheme: proxy.Scheme,
		User:   userinfo,
		Host:   proxy.Host + ":" + proxy.Port,
	}
	return out.String(), nil
}

// BuildProxyURL builds the final sticky proxy URL for one credential.
func BuildProxyURL(proxy ParsedProxy, platform, account string) (string, error) {
	platform = strings.TrimSpace(platform)
	account = strings.TrimSpace(account)
	if platform == "" {
		return "", fmt.Errorf("platform is empty")
	}
	if account == "" {
		return "", fmt.Errorf("account is empty")
	}
	userinfo := url.UserPassword(platform+"."+account, proxy.Password)
	out := &url.URL{
		Scheme: proxy.Scheme,
		User:   userinfo,
		Host:   proxy.Host + ":" + proxy.Port,
	}
	return out.String(), nil
}

// RedactProxyURL masks password material in proxy URLs for logs.
func RedactProxyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	name := u.User.Username()
	if _, ok := u.User.Password(); ok {
		// Avoid url.UserPassword encoding of "***" as %2A%2A%2A.
		return u.Scheme + "://" + name + ":***@" + u.Host
	}
	if name != "" {
		return u.Scheme + "://" + name + "@" + u.Host
	}
	return u.Scheme + "://" + u.Host
}

// BoolFromAny parses loose boolean values from config maps.
func BoolFromAny(v any, fallback bool) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		if s == "" {
			return fallback
		}
		b, err := strconv.ParseBool(s)
		if err != nil {
			return fallback
		}
		return b
	case float64:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	default:
		return fallback
	}
}

// IntFromAny parses loose integer values from config maps.
func IntFromAny(v any, fallback int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return fallback
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return fallback
		}
		return n
	default:
		return fallback
	}
}

// StringFromAny coerces map values to string.
func StringFromAny(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case fmt.Stringer:
		return strings.TrimSpace(t.String())
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strings.TrimSpace(strconv.FormatFloat(t, 'f', -1, 64))
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

// StringSliceFromAny coerces list-like config values.
func StringSliceFromAny(v any) []string {
	switch t := v.(type) {
	case []string:
		return cleanStringList(t)
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s := StringFromAny(item)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		if strings.Contains(s, ",") {
			parts := strings.Split(s, ",")
			return cleanStringList(parts)
		}
		return []string{s}
	default:
		return nil
	}
}

// StringMapFromAny coerces object-like config values.
func StringMapFromAny(v any) map[string]string {
	out := map[string]string{}
	switch t := v.(type) {
	case map[string]string:
		return cleanStringMap(t)
	case map[string]any:
		for k, val := range t {
			ks := strings.TrimSpace(k)
			vs := StringFromAny(val)
			if ks != "" && vs != "" {
				out[ks] = vs
			}
		}
	}
	return out
}

func cleanStringList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		s := strings.TrimSpace(item)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

func cleanStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		ks := strings.TrimSpace(k)
		vs := strings.TrimSpace(v)
		if ks == "" || vs == "" {
			continue
		}
		out[ks] = vs
	}
	return out
}
