package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(&Info{
		ID:   "test",
		Name: "Test Provider",
		Models: map[string]*Model{
			"model-1": {ID: "model-1", ProviderID: "test", Name: "Model 1"},
		},
	})

	p, err := r.GetProvider("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Test Provider" {
		t.Errorf("expected 'Test Provider', got %s", p.Name)
	}

	m, err := r.GetModel("test", "model-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "Model 1" {
		t.Errorf("expected 'Model 1', got %s", m.Name)
	}
}

func TestRegistry_GetModel_NotFound(t *testing.T) {
	r := NewRegistry()
	r.Register(&Info{
		ID:   "test",
		Name: "Test",
		Models: map[string]*Model{
			"llama3.3": {ID: "llama3.3", ProviderID: "test", Name: "Llama 3.3"},
		},
	})

	_, err := r.GetModel("test", "llama3")
	if err == nil {
		t.Fatal("expected error")
	}
	// Should suggest llama3.3
	if got := err.Error(); got == "" {
		t.Error("expected non-empty error message")
	}
}

func TestRegistry_GetProvider_NotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.GetProvider("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRegistry_Filters_Disabled(t *testing.T) {
	r := NewRegistry()
	r.SetFilters(nil, []string{"blocked"})

	r.Register(&Info{ID: "blocked", Name: "Blocked", Models: make(map[string]*Model)})
	r.Register(&Info{ID: "allowed", Name: "Allowed", Models: make(map[string]*Model)})

	if _, err := r.GetProvider("blocked"); err == nil {
		t.Error("blocked provider should not be registered")
	}
	if _, err := r.GetProvider("allowed"); err != nil {
		t.Errorf("allowed provider should be registered: %v", err)
	}
}

func TestRegistry_Filters_Enabled(t *testing.T) {
	r := NewRegistry()
	r.SetFilters([]string{"only-this"}, nil)

	r.Register(&Info{ID: "only-this", Name: "Allowed", Models: make(map[string]*Model)})
	r.Register(&Info{ID: "other", Name: "Other", Models: make(map[string]*Model)})

	if _, err := r.GetProvider("only-this"); err != nil {
		t.Errorf("should find enabled provider: %v", err)
	}
	if _, err := r.GetProvider("other"); err == nil {
		t.Error("non-enabled provider should be filtered out")
	}
}

func TestRegistry_ListModels(t *testing.T) {
	r := NewRegistry()
	r.Register(&Info{
		ID: "p1",
		Models: map[string]*Model{
			"m1": {ID: "m1", ProviderID: "p1"},
			"m2": {ID: "m2", ProviderID: "p1"},
		},
	})
	r.Register(&Info{
		ID: "p2",
		Models: map[string]*Model{
			"m3": {ID: "m3", ProviderID: "p2"},
		},
	})

	models := r.ListModels()
	if len(models) != 3 {
		t.Errorf("expected 3 models, got %d", len(models))
	}
}

func TestParseModel(t *testing.T) {
	tests := []struct {
		input      string
		providerID string
		modelID    string
	}{
		{"ollama/llama3", "ollama", "llama3"},
		{"openrouter/meta/model", "openrouter", "meta/model"},
		{"just-model", "", "just-model"},
	}
	for _, tt := range tests {
		pID, mID := ParseModel(tt.input)
		if pID != tt.providerID || mID != tt.modelID {
			t.Errorf("ParseModel(%q) = (%q, %q), want (%q, %q)",
				tt.input, pID, mID, tt.providerID, tt.modelID)
		}
	}
}

func TestParseModelSize(t *testing.T) {
	tests := []struct {
		name string
		want *float64
	}{
		{"llama3.3:70b", ptr(70.0)},
		{"qwen3.5:9b", ptr(9.0)},
		{"mistral-7B", ptr(7.0)},
		{"custom-model", nil},
	}
	for _, tt := range tests {
		got := parseModelSize(tt.name)
		if tt.want == nil {
			if got != nil {
				t.Errorf("parseModelSize(%q) = %v, want nil", tt.name, *got)
			}
		} else {
			if got == nil || *got != *tt.want {
				t.Errorf("parseModelSize(%q) = %v, want %v", tt.name, got, *tt.want)
			}
		}
	}
}

func TestProfileName(t *testing.T) {
	name := ProfileName("qwen3.5:9b", 18432)
	if name != "qwen3.5:9b-tc18k" {
		t.Errorf("expected qwen3.5:9b-tc18k, got %s", name)
	}
}

func TestIsProfile(t *testing.T) {
	if !IsProfile("qwen3.5:9b-tc18k") {
		t.Error("should recognize profile")
	}
	if IsProfile("qwen3.5:9b") {
		t.Error("should not match regular model")
	}
}

func TestBaseModelName(t *testing.T) {
	if got := BaseModelName("qwen3.5:9b-tc18k"); got != "qwen3.5:9b" {
		t.Errorf("expected qwen3.5:9b, got %s", got)
	}
}

func TestNumCtxFromProfileName(t *testing.T) {
	if got := NumCtxFromProfileName("qwen3.5:9b-tc18k"); got != 18*1024 {
		t.Errorf("expected %d, got %d", 18*1024, got)
	}
	if got := NumCtxFromProfileName("regular-model"); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestCalculateNumCtx(t *testing.T) {
	gpuMem := int64(16) * 1024 * 1024 * 1024 // 16 GB
	info := OllamaShowResult{
		ParameterSize:    "9B",
		QuantizationLevel: "Q4_K_M",
		BlockCount:       32,
		EmbeddingLength:  4096,
		HeadCount:        32,
		HeadCountKV:      8,
		ContextLength:    131072,
	}

	numCtx := CalculateNumCtx(gpuMem, info, 131072)
	if numCtx < minNumCtx || numCtx > maxNumCtx {
		t.Errorf("numCtx %d out of range [%d, %d]", numCtx, minNumCtx, maxNumCtx)
	}
	if numCtx%1024 != 0 {
		t.Errorf("numCtx %d not aligned to 1024", numCtx)
	}
}

func TestCalculateNumCtx_MissingArchInfo(t *testing.T) {
	info := OllamaShowResult{
		ParameterSize: "7B",
	}
	numCtx := CalculateNumCtx(16*1024*1024*1024, info, 32768)
	if numCtx != 8192 {
		t.Errorf("expected 8192 fallback, got %d", numCtx)
	}
}

func TestIsRetryable(t *testing.T) {
	retryable := []string{
		"fetch failed",
		"connection refused",
		"ECONNRESET",
		"rate limit exceeded",
		"too many requests",
		"server is overloaded, try again later",
		"at capacity",
	}
	for _, msg := range retryable {
		if !IsRetryable(msg) {
			t.Errorf("expected retryable: %q", msg)
		}
	}

	nonRetryable := []string{
		"invalid api key",
		"model not found",
		"bad request",
	}
	for _, msg := range nonRetryable {
		if IsRetryable(msg) {
			t.Errorf("expected non-retryable: %q", msg)
		}
	}
}

func TestIsRetryableStatus(t *testing.T) {
	if !IsRetryableStatus(429) {
		t.Error("429 should be retryable")
	}
	if !IsRetryableStatus(503) {
		t.Error("503 should be retryable")
	}
	if IsRetryableStatus(401) {
		t.Error("401 should not be retryable")
	}
}

func TestRetryDelay(t *testing.T) {
	d0 := RetryDelay(0)
	if d0 < RetryInitDelay/2 || d0 > RetryInitDelay*2 {
		t.Errorf("attempt 0 delay %v out of expected range", d0)
	}

	d4 := RetryDelay(4)
	if d4 > RetryMaxDelay*2 {
		t.Errorf("attempt 4 delay %v exceeds max", d4)
	}
}

func TestIsOverflow(t *testing.T) {
	overflow := []string{
		"prompt is too long",
		"maximum context length is 8192 tokens",
		"context_length_exceeded",
		"input length 50000 exceeds the context length 8192",
	}
	for _, msg := range overflow {
		if !IsOverflow(msg) {
			t.Errorf("expected overflow: %q", msg)
		}
	}
}

func TestDiscoverOllama(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		resp := ollamaTagsResponse{
			Models: []ollamaModel{
				{
					Name: "llama3.3:latest",
					Details: &ollamaModelDetails{
						ParameterSize: "70B",
						ContextLength: 131072,
						Family:        "llama",
					},
				},
				{
					Name:    "qwen3.5:9b-tc18k",
					Details: &ollamaModelDetails{Family: "qwen"},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.discoverOllama(t.Context(), srv.URL)

	p, err := reg.GetProvider("ollama")
	if err != nil {
		t.Fatalf("ollama provider not found: %v", err)
	}

	// Profile model should be filtered out
	if _, ok := p.Models["qwen3.5:9b-tc18k"]; ok {
		t.Error("profile model should be filtered from discovery")
	}

	m, err := reg.GetModel("ollama", "llama3.3:latest")
	if err != nil {
		t.Fatalf("model not found: %v", err)
	}
	if m.Limit.Context != 131072 {
		t.Errorf("expected context 131072, got %d", m.Limit.Context)
	}
}

func TestDiscoverVLLM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := vllmModelsResponse{
			Data: []vllmModel{
				{ID: "meta-llama/Llama-3-8B", MaxModelLen: 8192},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.discoverVLLM(t.Context(), srv.URL)

	m, err := reg.GetModel("vllm", "meta-llama/Llama-3-8B")
	if err != nil {
		t.Fatalf("model not found: %v", err)
	}
	if m.Limit.Context != 8192 {
		t.Errorf("expected context 8192, got %d", m.Limit.Context)
	}
}

func TestGPUMemoryBudget(t *testing.T) {
	// 16 GB → 8 GB budget
	budget := GPUMemoryBudget(16 * 1024 * 1024 * 1024)
	expected := int64(8) * 1024 * 1024 * 1024
	if budget != expected {
		t.Errorf("expected %d, got %d", expected, budget)
	}

	// 128 GB → capped at 32 GB
	budget = GPUMemoryBudget(128 * 1024 * 1024 * 1024)
	cap := int64(32) * 1024 * 1024 * 1024
	if budget != cap {
		t.Errorf("expected cap %d, got %d", cap, budget)
	}
}

func TestDiscovery_StartStop(t *testing.T) {
	reg := NewRegistry()
	b := bus.New()
	defer b.Close()

	d := NewDiscovery(reg, b)
	d.Start(t.Context(), "", "", "")

	time.Sleep(50 * time.Millisecond)
	d.Stop()
}

func ptr(f float64) *float64 {
	return &f
}
