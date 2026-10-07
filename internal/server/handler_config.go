package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
)

func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	dir := requestDirectory(r, ".")

	cfg, err := config.Load(dir)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if cfg.Model != "" {
		provID, modelID := provider.ParseModel(cfg.Model)
		if provID == "" || modelID == "" {
			if resolved := s.resolveDefaultModel(); resolved != nil {
				cfg.Model = resolved.ProviderID + "/" + resolved.ModelID
			}
		} else if _, err := s.deps.Registry.GetModel(provID, modelID); err != nil {
			if resolved := s.resolveDefaultModel(); resolved != nil {
				cfg.Model = resolved.ProviderID + "/" + resolved.ModelID
			}
		}
	}

	// Go has no share routes; empty share would show web SPA share UI. Default disabled.
	if cfg.Share == "" {
		cfg.Share = "disabled"
	}

	redactConfig(cfg)

	respondJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Normalize camelCase aliases to snake_case config file keys.
	if v, ok := body["topP"]; ok {
		if _, exists := body["top_p"]; !exists {
			body["top_p"] = v
		}
		delete(body, "topP")
	}
	if v, ok := body["maxTokens"]; ok {
		if _, exists := body["max_tokens"]; !exists {
			body["max_tokens"] = v
		}
		delete(body, "maxTokens")
	}

	configPath := config.GlobalConfigFile()

	existing := make(map[string]any)
	if data, err := os.ReadFile(configPath); err == nil {
		cleaned, err := config.ParseJSONC(string(data))
		if err == nil {
			_ = json.Unmarshal([]byte(cleaned), &existing)
		}
	}

	allowedFields := map[string]bool{
		"model": true, "theme": true, "logLevel": true, "small_model": true,
		"agents": true, "scopedModels": true, "temperature": true,
		"top_p": true, "max_tokens": true,
	}
	for k := range body {
		if !allowedFields[k] {
			respondError(w, http.StatusForbidden, fmt.Sprintf("field %q cannot be modified via API", k))
			return
		}
	}

	for k, v := range body {
		existing[k] = v
	}

	// Live-reload safe in-memory fields on the shared Config pointer.
	if s.deps.Config != nil {
		if v, ok := body["model"].(string); ok {
			s.deps.Config.Model = v
		}
		if v, ok := body["theme"].(string); ok {
			s.deps.Config.Theme = v
		}
		if v, ok := body["logLevel"].(string); ok {
			s.deps.Config.LogLevel = v
		}
		if v, ok := body["small_model"].(string); ok {
			s.deps.Config.SmallModel = v
		}
		if v, ok := asFloat64(body["temperature"]); ok {
			s.deps.Config.Temperature = &v
		}
		if v, ok := asFloat64(body["top_p"]); ok {
			s.deps.Config.TopP = &v
		}
		if v, ok := asInt(body["max_tokens"]); ok {
			s.deps.Config.MaxTokens = &v
		}
	}

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to marshal config")
		return
	}

	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create config directory")
		return
	}

	if err := os.WriteFile(configPath, append(data, '\n'), 0o600); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to write config")
		return
	}

	// Writes the global config file (config.GlobalConfigFile), not project-local.
	respondJSON(w, http.StatusOK, existing)
}

func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

// redactConfig strips sensitive values from cfg in place. The caller should
// pass a freshly-loaded config (not the shared in-memory pointer) so the
// original is never mutated.
func redactConfig(cfg *config.Info) {
	const redacted = "[REDACTED]"

	for name, mcp := range cfg.MCP {
		for k := range mcp.Headers {
			mcp.Headers[k] = redacted
		}
		for k := range mcp.Env {
			mcp.Env[k] = redacted
		}
		if mcp.OAuth != nil {
			mcp.OAuth.AccessToken = redacted
		}
		cfg.MCP[name] = mcp
	}

	for name, prov := range cfg.Provider {
		for k := range prov.Headers {
			prov.Headers[k] = redacted
		}
		for _, key := range []string{"api_key", "apiKey", "authorization", "Authorization"} {
			if _, ok := prov.Options[key]; ok {
				prov.Options[key] = redacted
			}
		}
		cfg.Provider[name] = prov
	}
}

func (s *Server) handleConfigProviders(w http.ResponseWriter, r *http.Request) {
	providers := s.deps.Registry.ListProviders()

	type providerSummary struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Source string `json:"source"`
		Models int    `json:"models"`
	}

	result := make([]providerSummary, 0, len(providers))
	for _, p := range providers {
		result = append(result, providerSummary{
			ID:     p.ID,
			Name:   p.Name,
			Source: p.Source,
			Models: len(p.Models),
		})
	}

	respondJSON(w, http.StatusOK, result)
}
