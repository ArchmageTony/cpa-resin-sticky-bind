package stickybind

import (
	"net/url"
	"strings"
)

// ResolveProxyInput returns the raw configured proxy URL for one credential,
// picking proxy_by_type[type] first, then other_proxy_url. An empty result
// means the caller falls back to the legacy resin_proxy_url path.
// Type lookup is case-insensitive.
func (c Config) ResolveProxyInput(provider string) string {
	if v := lookupTypeValue(c.ProxyByType, provider); v != "" {
		return v
	}
	return strings.TrimSpace(c.OtherProxyURL)
}

// TypeProxyURL returns the dedicated proxy_by_type entry for one type
// (case-insensitive), without other_proxy_url fallback. Used for display.
func (c Config) TypeProxyURL(provider string) string {
	return lookupTypeValue(c.ProxyByType, provider)
}

// lookupTypeValue does a case-insensitive map lookup for trimmed provider.
func lookupTypeValue(m map[string]string, provider string) string {
	if len(m) == 0 {
		return ""
	}
	key := strings.TrimSpace(provider)
	if key == "" {
		return ""
	}
	if v, ok := m[key]; ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	lower := strings.ToLower(key)
	for k, v := range m {
		if strings.ToLower(strings.TrimSpace(k)) == lower && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// RedactTypedProxyURL masks the token of a per-type proxy entry for page and
// log rendering. Username:token forms keep the identity and mask the token;
// username-only token forms mask the whole userinfo.
func RedactTypedProxyURL(raw string) string {
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
		return u.Scheme + "://" + name + ":***@" + u.Host
	}
	if name != "" {
		return u.Scheme + "://***@" + u.Host
	}
	return u.Scheme + "://" + u.Host
}
