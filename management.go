package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ArchmageTony/cpa-resin-sticky-bind/internal/stickybind"
)

const (
	resourcePath        = "/status"
	resourceContentType = "text/html; charset=utf-8"
)

type managementRegistration struct {
	Routes    []managementRoute    `json:"routes,omitempty"`
	Resources []managementResource `json:"resources,omitempty"`
}

// Use exported field names (no custom json tags) so CPA host decoding matches pluginapi.ResourceRoute.
type managementResource struct {
	Path        string
	Menu        string
	Description string
}

type managementRoute struct {
	Method      string
	Path        string
	Menu        string
	Description string
}

type managementRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Query   url.Values
	Body    []byte
}

type managementResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

func buildManagementRegistration() managementRegistration {
	return managementRegistration{
		// Browser resource under /v0/resource/plugins/<pluginID>/status
		Resources: []managementResource{{
			Path:        resourcePath,
			Menu:        "Resin Sticky Bind",
			Description: "Status and manual sync for per-auth Resin sticky proxy binding.",
		}},
		// Legacy GET+Menu also maps into resource routes on some CPA builds.
		Routes: []managementRoute{{
			Method:      http.MethodGet,
			Path:        resourcePath,
			Menu:        "Resin Sticky Bind",
			Description: "Status and manual sync for per-auth Resin sticky proxy binding.",
		}},
	}
}

func handleManagement(raw []byte) ([]byte, error) {
	var req managementRequest
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("decode management request: %w", err)
		}
	}
	// Host passes the full request path, e.g. /v0/resource/plugins/cpa-resin-sticky-bind/status
	if !isStatusResourcePath(req.Path) {
		return okEnvelope(htmlResponse(http.StatusNotFound, "<html><body>not found</body></html>"))
	}

	cfg := getRuntime().Config()
	action := ""
	if req.Query != nil {
		action = strings.ToLower(strings.TrimSpace(req.Query.Get("action")))
	}
	var syncNote string
	if action == "sync" {
		wrote, skipped, unchanged, failed, err := runManualSync(cfg)
		if err != nil {
			syncNote = "sync error: " + err.Error()
		} else {
			syncNote = fmt.Sprintf("手动同步完成: 写入 %d / 跳过 %d / 未变化 %d / 失败 %d", wrote, skipped, unchanged, failed)
		}
	}

	page := renderStatusPage(cfg, syncNote, collectProviderCounts())
	return okEnvelope(htmlResponse(http.StatusOK, page))
}

// collectProviderCounts counts persisted credentials per provider type for
// the management page. Errors yield an empty map: the page stays usable.
func collectProviderCounts() map[string]int {
	entries, err := (hostAdapter{}).ListAuths()
	if err != nil {
		return map[string]int{}
	}
	counts := map[string]int{}
	for _, entry := range entries {
		if entry.RuntimeOnly {
			continue
		}
		provider := firstNonEmptyString(entry.Provider, entry.Type)
		if strings.TrimSpace(provider) == "" {
			continue
		}
		counts[strings.ToLower(strings.TrimSpace(provider))]++
	}
	return counts
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func isStatusResourcePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" || path == resourcePath {
		return true
	}
	path = strings.TrimRight(path, "/")
	return strings.HasSuffix(path, resourcePath) || strings.HasSuffix(path, "/status")
}

func runManualSync(cfg stickybind.Config) (wrote, skipped, unchanged, failed int, err error) {
	b := &stickybind.Binder{
		Cfg:    cfg,
		Host:   hostAdapter{},
		Getenv: os.Getenv,
	}
	return b.SyncOnce()
}

func htmlResponse(status int, body string) managementResponse {
	headers := make(http.Header)
	headers.Set("Content-Type", resourceContentType)
	return managementResponse{
		StatusCode: status,
		Headers:    headers,
		Body:       []byte(body),
	}
}
