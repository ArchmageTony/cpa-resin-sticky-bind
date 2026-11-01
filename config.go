package main

import (
	"github.com/ArchmageTony/cpa-resin-sticky-bind/internal/stickybind"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func configDefaults() stickybind.Config {
	return stickybind.Defaults()
}

func configFields() []pluginapi.ConfigField {
	return []pluginapi.ConfigField{
		{Name: "enabled", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Enable sticky Resin proxy binding"},
		{Name: "resin_proxy_url", Type: pluginapi.ConfigFieldTypeString, Description: "Deprecated legacy fallback proxy URL, only used when other_proxy_url is empty"},
		{Name: "proxy_token_env", Type: pluginapi.ConfigFieldTypeString, Description: "Deprecated env var fallback for the legacy chain token"},
		{Name: "default_platform", Type: pluginapi.ConfigFieldTypeString, Description: "Default Resin Platform"},
		{Name: "platform_by_provider", Type: pluginapi.ConfigFieldTypeObject, Description: "Optional Platform override map keyed by provider"},
		{Name: "platform_by_auth_id", Type: pluginapi.ConfigFieldTypeObject, Description: "Optional Platform override map keyed by auth id"},
		{Name: "account_strategy", Type: pluginapi.ConfigFieldTypeEnum, EnumValues: []string{"auth_id", "email", "sub", "filename"}, Description: "Account identity strategy"},
		{Name: "account_prefix", Type: pluginapi.ConfigFieldTypeString, Description: "Optional Account prefix"},
		{Name: "sync_interval_seconds", Type: pluginapi.ConfigFieldTypeNumber, Description: "Credential reconcile interval in seconds"},
		{Name: "only_if_empty", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Only write when proxy_url is empty"},
		{Name: "overwrite_existing", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Allow overwriting existing proxy_url"},
		{Name: "include_providers", Type: pluginapi.ConfigFieldTypeArray, Description: "Only process these providers (empty = all)"},
		{Name: "exclude_providers", Type: pluginapi.ConfigFieldTypeArray, Description: "Skip these providers"},
		{Name: "proxy_by_type", Type: pluginapi.ConfigFieldTypeObject, Description: "Full proxy URL per credential type, Resin sticky form supported"},
		{Name: "sticky_by_type", Type: pluginapi.ConfigFieldTypeObject, Description: "Per-type sticky expansion flag, key 'other' for the fallback row (default true)"},
		{Name: "other_proxy_url", Type: pluginapi.ConfigFieldTypeString, Description: "Fallback proxy URL for credential types without a dedicated entry"},
		{Name: "expand_sticky_account", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Default sticky expansion for types not listed in sticky_by_type"},
		{Name: "rebind_on_change", Type: pluginapi.ConfigFieldTypeBoolean, Description: "Rewrite existing proxy_url when the resolved target changes"},
	}
}

func parseConfigFromReconfigure(request []byte) stickybind.Config {
	return stickybind.ParseConfigFromRequest(request)
}
