package stickybind

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"unicode"
)

// AccountInput carries candidate identity fields for one credential.
type AccountInput struct {
	AuthID   string
	Email    string
	Sub      string
	Filename string
	Provider string
}

// ResolvePlatform picks platform by auth id, provider, then default.
func ResolvePlatform(cfg Config, authID, provider string) string {
	cfg = cfg.Normalize()
	authID = strings.TrimSpace(authID)
	provider = strings.TrimSpace(provider)
	if authID != "" {
		if p, ok := cfg.PlatformByAuthID[authID]; ok && strings.TrimSpace(p) != "" {
			return strings.TrimSpace(p)
		}
	}
	if provider != "" {
		if p, ok := cfg.PlatformByProvider[provider]; ok && strings.TrimSpace(p) != "" {
			return strings.TrimSpace(p)
		}
		// case-insensitive provider match
		lower := strings.ToLower(provider)
		for k, v := range cfg.PlatformByProvider {
			if strings.ToLower(strings.TrimSpace(k)) == lower && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return strings.TrimSpace(cfg.DefaultPlatform)
}

// BuildAccount generates a stable Resin account from credential identity.
// The result is always ASCII: non-ASCII input collapses to underscores plus
// a deterministic hash suffix so accounts stay unique and the rendered
// proxy URL stays parseable.
func BuildAccount(cfg Config, in AccountInput) string {
	cfg = cfg.Normalize()
	raw := pickAccountRaw(cfg.AccountStrategy, in)
	base, nonASCII := sanitizeAccount(raw)
	if base == "" {
		base = shortHash(raw)
	} else if nonASCII {
		base = base + "_" + shortHash(raw)
	}
	if cfg.AccountPrefix != "" {
		prefix, _ := sanitizeAccount(cfg.AccountPrefix)
		if prefix != "" {
			if strings.HasSuffix(prefix, "_") || strings.HasSuffix(prefix, "-") || strings.HasSuffix(prefix, ".") {
				base = prefix + base
			} else {
				base = prefix + "_" + base
			}
		}
	}
	return fitAccount(base, raw)
}

func pickAccountRaw(strategy string, in AccountInput) string {
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	ordered := make([]string, 0, 4)
	switch strategy {
	case AccountStrategyEmail:
		ordered = []string{in.Email, in.AuthID, in.Sub, in.Filename}
	case AccountStrategySub:
		ordered = []string{in.Sub, in.AuthID, in.Email, in.Filename}
	case AccountStrategyFilename:
		ordered = []string{in.Filename, in.AuthID, in.Email, in.Sub}
	default: // auth_id
		ordered = []string{in.AuthID, in.Email, in.Sub, in.Filename}
	}
	for _, item := range ordered {
		item = strings.TrimSpace(item)
		if item != "" {
			return item
		}
	}
	return "unknown"
}

// sanitizeAccount folds a raw identifier into the Resin account charset.
// Only ASCII letters, digits and . _ - are kept: accounts must stay ASCII so
// the rendered proxy URL is always parseable and comparisons are stable.
// Non-ASCII input collapses like any other invalid character; the caller
// appends a deterministic hash suffix to preserve uniqueness.
func sanitizeAccount(raw string) (base string, nonASCII bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	// strip path if filename-like
	raw = filepath.Base(raw)
	raw = strings.ToLower(raw)
	var b strings.Builder
	b.Grow(len(raw))
	lastUnderscore := false
	for _, r := range raw {
		if r > 0x7F {
			nonASCII = true
			r = ' '
		}
		ok := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "._-")
	return out, nonASCII
}

func fitAccount(base, raw string) string {
	if base == "" {
		return shortHash(raw)
	}
	if len(base) <= MaxAccountLen {
		return base
	}
	hash := shortHash(raw)
	// keep head + hash, total <= 64
	keep := MaxAccountLen - 1 - len(hash)
	if keep < 8 {
		return hash
	}
	return base[:keep] + "_" + hash
}

func shortHash(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:8])
}

// ProviderAllowed reports whether provider passes include/exclude filters.
func ProviderAllowed(cfg Config, provider string) bool {
	cfg = cfg.Normalize()
	provider = strings.TrimSpace(provider)
	if provider == "" {
		// empty provider still allowed unless include list is non-empty
		if len(cfg.IncludeProviders) > 0 {
			return false
		}
		return true
	}
	lower := strings.ToLower(provider)
	if len(cfg.IncludeProviders) > 0 {
		ok := false
		for _, item := range cfg.IncludeProviders {
			if strings.ToLower(item) == lower {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, item := range cfg.ExcludeProviders {
		if strings.ToLower(item) == lower {
			return false
		}
	}
	return true
}

// ExtractAccountFromProxyURL returns Platform and Account from username Platform.Account.
func ExtractAccountFromProxyURL(proxyURL string) (platform, account string, ok bool) {
	u, err := parseLooseURL(proxyURL)
	if err != nil || u.User == nil {
		return "", "", false
	}
	user := strings.TrimSpace(u.User.Username())
	if user == "" {
		return "", "", false
	}
	parts := strings.SplitN(user, ".", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	platform = strings.TrimSpace(parts[0])
	account = strings.TrimSpace(parts[1])
	if platform == "" || account == "" {
		return "", "", false
	}
	return platform, account, true
}
