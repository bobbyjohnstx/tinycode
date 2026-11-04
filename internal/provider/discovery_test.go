package provider

import "testing"

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
