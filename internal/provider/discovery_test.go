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
		// Granite versioned contexts
		{"granite-3-1-8b-instruct", 4096},
		{"granite-3.1-8b-instruct", 4096},
		{"granite-3-2-8b-instruct", 60000},
		{"granite-3.2-8b-instruct", 60000},
		{"granite-3-3-8b-instruct", 131000},
		{"granite-4-0-h-tiny", 131000},
		{"granite-4-1-8b", 131000},
		{"granite-4-1-30b", 131000},
		{"granite-guardian-3-3-8b", 32768},
		{"granite-guardian-4-1-8b", 32768},
		// Claude
		{"claude-sonnet-5.5", 200000},
		{"claude-haiku-5-5", 200000},
		// Qwen
		{"qwen3-8-27b-fp8", 262000},
		{"qwen3.5-9b", 262000},
		{"qwen3-reranker-4b", 32768},
		// Gemini
		{"gemini-2-5-flash", 1049000},
		{"gemini-3-7-flash", 1049000},
		// Gemma
		{"gemma-4-31b-it", 262000},
		{"diffusiongemma-26b-a4b-it-fp8", 262000},
		// Nemotron
		{"nemotron-nano-30b-a3b-fp8", 262000},
		{"nemotron-super-120b-bf16-ga", 262000},
		{"nemotron-3-5-parse", 16384},
		{"nvidia-nemotron-3-ultra-550b-a55b-nvfp4", 262000},
		// GPT-OSS
		{"gpt-oss-120b", 131000},
		{"gpt-oss-20b", 131000},
		// Llama
		{"llama-3-3-70b-instruct", 131000},
		// Mistral
		{"mistral-7b-instruct-v0-3", 33000},
		{"mistral-small-3-2-24b-instruct-2506", 131000},
		// Other
		{"deepseek-4-1-flash", 65536},
		{"phi-3-mini", 16384},
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
