package stickybind

import (
	"fmt"
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

// ProxyAccountPlaceholder marks an account-bound (sticky) proxy entry: every
// occurrence is replaced with the per-credential account derived from the
// configured account strategy. Entries without the placeholder are used
// verbatim and shared by every credential of that type.
const ProxyAccountPlaceholder = "{account}"

// NormalizeProxyEntry validates one proxy_by_type / other_proxy_url entry and
// reports whether it is account-bound (sticky). The only semantic is: an
// entry containing {account} is a per-credential template, anything else is
// used verbatim. Validation keeps our own sanity rules: parseable URL,
// known scheme, host required. Validation runs against a placeholder-free
// copy so curly braces never confuse the URL parser; the returned entry
// preserves the original text.
func NormalizeProxyEntry(raw string) (entry string, accountBound bool, err error) {
	entry = strings.TrimSpace(raw)
	if entry == "" {
		return "", false, fmt.Errorf("proxy url is required")
	}
	accountBound = strings.Contains(entry, ProxyAccountPlaceholder)
	probe := strings.ReplaceAll(entry, ProxyAccountPlaceholder, "account")
	parsed, errParse := url.Parse(probe)
	if errParse != nil || parsed.Scheme == "" {
		return "", false, fmt.Errorf("proxy url format is invalid")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "socks5", "socks5h", "http", "https":
	default:
		return "", false, fmt.Errorf("proxy url scheme must be socks5, socks5h, http, or https")
	}
	if parsed.Host == "" || parsed.Hostname() == "" {
		return "", false, fmt.Errorf("proxy url host is required")
	}
	return entry, accountBound, nil
}

// RenderProxyEntry substitutes every {account} placeholder in an
// account-bound entry. The account is percent-encoded with userinfo rules
// first (same as url.UserPassword) so non-ASCII accounts — auth ids may fall
// back to non-ASCII file names — keep the final proxy URL parseable; proxy
// dialers decode the username again before sending it upstream. Entries
// without the placeholder are returned as-is.
func RenderProxyEntry(normalized string, account string) string {
	if !strings.Contains(normalized, ProxyAccountPlaceholder) {
		return normalized
	}
	escaped := url.User(strings.TrimSpace(account)).String()
	return strings.ReplaceAll(normalized, ProxyAccountPlaceholder, escaped)
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
// username-only token forms mask the whole userinfo. Account-bound entries
// carry the {account} placeholder, which is not URL-safe: it is temporarily
// percent-encoded so the URL parser accepts it, and Username() decodes it
// back, so the rendered mask keeps the placeholder visible.
func RedactTypedProxyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	probe := strings.ReplaceAll(raw, ProxyAccountPlaceholder, "%7Baccount%7D")
	u, err := url.Parse(probe)
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
