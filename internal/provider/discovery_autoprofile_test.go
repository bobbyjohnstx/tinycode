package provider

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
)

func TestBuildOllamaModels_ShowModelContextWhenTagsOmit(t *testing.T) {
	var mu sync.Mutex
	created := make(map[string]int)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"details": map[string]any{
					"parameter_size":     "8B",
					"quantization_level": "Q4_K_M",
				},
				"model_info": map[string]any{
					"llama.context_length":   32768,
					"llama.block_count":      32,
					"llama.embedding_length": 4096,
					"llama.attention.head_count":    32,
					"llama.attention.head_count_kv": 8,
				},
			})
		case "/api/create":
			var body struct {
				Model      string         `json:"model"`
				Parameters map[string]int `json:"parameters"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			created[body.Model] = body.Parameters["num_ctx"]
			mu.Unlock()
			_, _ = io.WriteString(w, `{"status":"success"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()
	d := NewDiscovery(reg, b)
	enabled := true
	d.SetAutoProfile(&AutoProfileConfig{Enabled: &enabled})
	d.detectGPU = func() (int64, error) {
		return 24 * 1024 * 1024 * 1024, nil // 24GB → room for >8k ctx
	}

	tags := ollamaTagsResponse{
		Models: []ollamaModel{
			{Name: "qwen3:8b"}, // no details.context_length
		},
	}
	models := d.buildOllamaModels(t.Context(), srv.URL, tags)
	m := models["qwen3:8b"]
	if m == nil {
		t.Fatal("expected model qwen3:8b")
	}
	if !strings.Contains(m.API.ID, "-tc") {
		t.Fatalf("expected profile API id, got %q", m.API.ID)
	}
	if m.Limit.Context <= 8192 {
		t.Fatalf("profile context = %d, want > 8192 from ShowModel", m.Limit.Context)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(created) == 0 {
		t.Fatal("expected CreateProfile call")
	}
	for name, numCtx := range created {
		if numCtx <= 8192 {
			t.Fatalf("created %s with num_ctx=%d, want > 8192", name, numCtx)
		}
	}
}

func TestCreateProfile_SurvivesSlowResponseBeyondProbeTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/create" {
			http.NotFound(w, r)
			return
		}
		time.Sleep(3 * time.Second) // longer than probeTimeout (2s)
		_, _ = io.WriteString(w, `{"status":"success"}`)
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()
	d := NewDiscovery(reg, b)

	start := time.Now()
	err := CreateProfile(t.Context(), d.ollamaClient, srv.URL, "base", "base-tc16k", 16384)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("CreateProfile with ollamaClient failed: %v", err)
	}
	if elapsed < 3*time.Second {
		t.Fatalf("expected slow create to take >=3s, took %v", elapsed)
	}

	// Probe client must still time out under 2s+slack on the same slow endpoint.
	err = CreateProfile(t.Context(), d.client, srv.URL, "base", "base-tc16k", 16384)
	if err == nil {
		t.Fatal("expected probe client (2s) to fail on 3s create")
	}
}

func TestBuildOllamaModels_DeletesSupersededProfile(t *testing.T) {
	var mu sync.Mutex
	var deleted []string
	var created []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/show":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"details": map[string]any{
					"parameter_size":     "8B",
					"quantization_level": "Q4_K_M",
				},
				"model_info": map[string]any{
					"llama.context_length":          32768,
					"llama.block_count":             32,
					"llama.embedding_length":        4096,
					"llama.attention.head_count":    32,
					"llama.attention.head_count_kv": 8,
				},
			})
		case "/api/create":
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			created = append(created, body.Model)
			mu.Unlock()
			_, _ = io.WriteString(w, `{"status":"success"}`)
		case "/api/delete":
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			deleted = append(deleted, body.Model)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := NewRegistry()
	b := bus.New()
	defer b.Close()
	d := NewDiscovery(reg, b)
	enabled := true
	numCtx := 16384
	d.SetAutoProfile(&AutoProfileConfig{
		Enabled:       &enabled,
		DefaultNumCtx: &numCtx,
	})

	tags := ollamaTagsResponse{
		Models: []ollamaModel{
			{Name: "qwen3:8b"},
			{Name: "qwen3:8b-tc8k"}, // old profile for same base
		},
	}
	_ = d.buildOllamaModels(t.Context(), srv.URL, tags)

	mu.Lock()
	defer mu.Unlock()
	foundDelete := false
	for _, name := range deleted {
		if name == "qwen3:8b-tc8k" {
			foundDelete = true
		}
	}
	if !foundDelete {
		t.Fatalf("expected delete of superseded profile qwen3:8b-tc8k, got %v", deleted)
	}
	wantCreate := ProfileName("qwen3:8b", numCtx)
	foundCreate := false
	for _, name := range created {
		if name == wantCreate {
			foundCreate = true
		}
	}
	if !foundCreate {
		t.Fatalf("expected create of %s, got %v", wantCreate, created)
	}
}
