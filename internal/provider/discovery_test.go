package provider

import (
	"testing"
)

func TestNormalizeModelKey(t *testing.T) {
	tests := []struct {
		a, b string
	}{
		{"granite-4.0-h-tiny", "granite-4-0-h-tiny"},
		{"granite-3.2-8b-instruct", "granite-3-2-8b-instruct"},
		{"granite-4.0-micro", "granite-4-0-micro"},
		{"nomic-embed-text-v1.5", "nomic-embed-text-v1-5"},
		{"granite-guardian-3.3-8b", "granite-guardian-3-3-8b"},
		{"granite-4.1-8b", "granite-4-1-8b"},
	}
	for _, tt := range tests {
		ka := normalizeModelKey(tt.a)
		kb := normalizeModelKey(tt.b)
		if ka != kb {
			t.Errorf("normalizeModelKey(%q) = %q, normalizeModelKey(%q) = %q — should match", tt.a, ka, tt.b, kb)
		}
	}
}

func TestNormalizeModelKey_DistinctModelsStayDistinct(t *testing.T) {
	a := normalizeModelKey("granite-3.1-8b-instruct")
	b := normalizeModelKey("granite-3.2-8b-instruct")
	if a == b {
		t.Errorf("different models should produce different keys: %q vs %q", a, b)
	}
}

func TestInferContextFromModelID(t *testing.T) {
	tests := []struct {
		id   string
		want int
	}{
		{"granite-3.1-8b-instruct", 128000},
		{"claude-sonnet-5.5", 200000},
		{"qwen3.5-9b", 32768},
		{"llama-3.1-70b", 128000},
		{"mistral-7b-instruct", 32768},
		{"deepseek-v4-flash", 65536},
		{"phi-3-mini", 16384},
		{"gemma-2-9b", 8192},
		{"nvidia-nemotron-3-ultra-550b", 128000},
		{"unknown-model-v1", 32768},
		{"", 32768},
	}
	for _, tt := range tests {
		got := inferContextFromModelID(tt.id)
		if got != tt.want {
			t.Errorf("inferContextFromModelID(%q) = %d, want %d", tt.id, got, tt.want)
		}
	}
}
