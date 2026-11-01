package stickybind

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseConfigFromRequest parses plugin.register / plugin.reconfigure payload.
func ParseConfigFromRequest(request []byte) Config {
	cfg := Defaults()
	if len(request) == 0 {
		return cfg.Normalize()
	}
	var raw map[string]any
	if err := json.Unmarshal(request, &raw); err != nil {
		return cfg.Normalize()
	}
	if yamlBytes, ok := extractYAMLBytes(raw); ok {
		applyYAMLConfig(&cfg, yamlBytes)
		return cfg.Normalize()
	}
	configMap := raw
	if nested, ok := raw["config"].(map[string]any); ok {
		configMap = nested
	}
	applyConfigMap(&cfg, configMap)
	return cfg.Normalize()
}

func extractYAMLBytes(raw map[string]any) ([]byte, bool) {
	v, ok := raw["config_yaml"]
	if !ok || v == nil {
		// also accept camelCase from some hosts
		v, ok = raw["configYAML"]
		if !ok || v == nil {
			return nil, false
		}
	}
	switch t := v.(type) {
	case string:
		if decoded, err := base64.StdEncoding.DecodeString(t); err == nil && looksLikeYAML(decoded) {
			return decoded, true
		}
		return []byte(t), true
	case []byte:
		return t, true
	default:
		return nil, false
	}
}

func looksLikeYAML(raw []byte) bool {
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return false
	}
	return strings.Contains(s, ":") || strings.HasPrefix(s, "---")
}

func applyYAMLConfig(cfg *Config, yamlBytes []byte) {
	var m map[string]any
	if err := yaml.Unmarshal(yamlBytes, &m); err != nil {
		return
	}
	applyConfigMap(cfg, m)
}

func applyConfigMap(cfg *Config, m map[string]any) {
	if cfg == nil || m == nil {
		return
	}
	if v, ok := m["enabled"]; ok {
		cfg.Enabled = BoolFromAny(v, cfg.Enabled)
	}
	if v, ok := m["resin_proxy_url"]; ok {
		cfg.ResinProxyURL = StringFromAny(v)
	}
	if v, ok := m["proxy_token_env"]; ok {
		cfg.ProxyTokenEnv = StringFromAny(v)
	}
	if v, ok := m["default_platform"]; ok {
		cfg.DefaultPlatform = StringFromAny(v)
	}
	if v, ok := m["platform_by_provider"]; ok {
		cfg.PlatformByProvider = StringMapFromAny(v)
	}
	if v, ok := m["platform_by_auth_id"]; ok {
		cfg.PlatformByAuthID = StringMapFromAny(v)
	}
	if v, ok := m["account_strategy"]; ok {
		cfg.AccountStrategy = StringFromAny(v)
	}
	if v, ok := m["account_prefix"]; ok {
		cfg.AccountPrefix = StringFromAny(v)
	}
	if v, ok := m["sync_interval_seconds"]; ok {
		cfg.SyncIntervalSeconds = IntFromAny(v, cfg.SyncIntervalSeconds)
	}
	if v, ok := m["only_if_empty"]; ok {
		cfg.OnlyIfEmpty = BoolFromAny(v, cfg.OnlyIfEmpty)
	}
	if v, ok := m["overwrite_existing"]; ok {
		cfg.OverwriteExisting = BoolFromAny(v, cfg.OverwriteExisting)
	}
	if v, ok := m["include_providers"]; ok {
		cfg.IncludeProviders = StringSliceFromAny(v)
	}
	if v, ok := m["exclude_providers"]; ok {
		cfg.ExcludeProviders = StringSliceFromAny(v)
	}
	if v, ok := m["proxy_by_type"]; ok {
		cfg.ProxyByType = StringMapFromAny(v)
	}
	if v, ok := m["other_proxy_url"]; ok {
		cfg.OtherProxyURL = StringFromAny(v)
	}
	if v, ok := m["rebind_on_change"]; ok {
		cfg.RebindOnChange = BoolFromAny(v, cfg.RebindOnChange)
	}
}
