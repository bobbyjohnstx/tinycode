package server

import (
	"testing"

	"github.com/bobbyjohnstx/tinycode/internal/config"
)

func TestRedactConfig_ProviderOptionsSecrets(t *testing.T) {
	cfg := &config.Info{
		Provider: map[string]config.ProviderConfig{
			"openai": {
				Options: map[string]any{
					"api_key":         "sk-underscore",
					"apiKey":          "sk-camel",
					"authorization":   "Bearer lower",
					"Authorization":   "Bearer Upper",
					"baseURL":         "https://api.openai.com",
				},
			},
		},
	}

	redactConfig(cfg)

	opts := cfg.Provider["openai"].Options
	for _, key := range []string{"api_key", "apiKey", "authorization", "Authorization"} {
		if opts[key] != "[REDACTED]" {
			t.Errorf("expected %s redacted, got %v", key, opts[key])
		}
	}
	if opts["baseURL"] != "https://api.openai.com" {
		t.Errorf("expected baseURL preserved, got %v", opts["baseURL"])
	}
}
