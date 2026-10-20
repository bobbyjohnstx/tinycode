package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/config"
)

// remoteCatalog holds the last successfully loaded catalog for late Register merges.
var remoteCatalog atomic.Pointer[catalogFile]

// catalogFile is models.dev-style provider → models map.
type catalogFile map[string]catalogProvider

type catalogProvider struct {
	ID     string                   `json:"id"`
	Name   string                   `json:"name"`
	Models map[string]catalogModel  `json:"models"`
}

type catalogCost struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

type catalogLimit struct {
	Context int `json:"context"`
	Output  int `json:"output"`
}

type catalogModalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type catalogModel struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Family      string             `json:"family"`
	ReleaseDate string             `json:"release_date"`
	Attachment  bool               `json:"attachment"`
	Reasoning   bool               `json:"reasoning"`
	Temperature bool               `json:"temperature"`
	ToolCall    bool               `json:"tool_call"`
	Cost        *catalogCost       `json:"cost"`
	Limit       *catalogLimit      `json:"limit"`
	Modalities  *catalogModalities `json:"modalities"`
}

// LoadCatalog loads a remote/local model catalog (fail-open).
// Priority: TINYCODE_MODELS_PATH file → fetch TINYCODE_MODELS_URL/api.json (unless DISABLE).
func LoadCatalog(ctx context.Context) (catalogFile, error) {
	if path := os.Getenv("TINYCODE_MODELS_PATH"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading TINYCODE_MODELS_PATH: %w", err)
		}
		var cat catalogFile
		if err := json.Unmarshal(data, &cat); err != nil {
			return nil, fmt.Errorf("parsing catalog path: %w", err)
		}
		return cat, nil
	}

	base := strings.TrimRight(os.Getenv("TINYCODE_MODELS_URL"), "/")
	if base == "" {
		return nil, nil
	}
	if os.Getenv("TINYCODE_DISABLE_MODELS_FETCH") != "" {
		return loadCachedCatalog(base)
	}

	url := base + "/api.json"
	cat, err := fetchCatalog(ctx, url)
	if err != nil {
		slog.Warn("model catalog fetch failed; using cache if any", "url", url, "error", err)
		if cached, cerr := loadCachedCatalog(base); cerr == nil && cached != nil {
			return cached, nil
		}
		return nil, err
	}
	_ = writeCachedCatalog(base, cat)
	return cat, nil
}

func fetchCatalog(ctx context.Context, url string) (catalogFile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "tinycode")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var cat catalogFile
	if err := json.Unmarshal(body, &cat); err != nil {
		return nil, err
	}
	return cat, nil
}

func catalogCachePath(baseURL string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, baseURL)
	return filepath.Join(config.DataDir(), "cache", "models-"+safe+".json")
}

func writeCachedCatalog(baseURL string, cat catalogFile) error {
	path := catalogCachePath(baseURL)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(cat)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadCachedCatalog(baseURL string) (catalogFile, error) {
	data, err := os.ReadFile(catalogCachePath(baseURL))
	if err != nil {
		return nil, err
	}
	var cat catalogFile
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, err
	}
	return cat, nil
}

// MergeCatalogIntoRegistry fills Limit/Cost/Capabilities/ReleaseDate for models
// that already exist (exact provider+model ID). Discovery wins existence — catalog
// never registers new models.
func MergeCatalogIntoRegistry(reg *Registry, cat catalogFile) int {
	if reg == nil || cat == nil {
		return 0
	}
	merged := 0
	for providerID, prov := range cat {
		if providerID == "" {
			providerID = prov.ID
		}
		info, err := reg.GetProvider(providerID)
		if err != nil || info == nil {
			continue
		}
		for modelID, cm := range prov.Models {
			if modelID == "" {
				modelID = cm.ID
			}
			m, ok := info.Models[modelID]
			if !ok {
				// Also try provider/model compound keys used by some discoveries.
				compound := providerID + "/" + modelID
				m, ok = info.Models[compound]
				if !ok {
					continue
				}
			}
			applyCatalogModel(m, cm)
			merged++
		}
	}
	return merged
}

func applyCatalogModel(m *Model, cm catalogModel) {
	if cm.Name != "" && m.Name == "" {
		m.Name = cm.Name
	}
	if cm.Family != "" {
		m.Family = cm.Family
	}
	if cm.ReleaseDate != "" {
		m.ReleaseDate = cm.ReleaseDate
	}
	if cm.Limit != nil {
		if cm.Limit.Context > 0 {
			m.Limit.Context = cm.Limit.Context
		}
		if cm.Limit.Output > 0 {
			m.Limit.Output = cm.Limit.Output
		}
	}
	if cm.Cost != nil {
		m.Cost.Input = cm.Cost.Input
		m.Cost.Output = cm.Cost.Output
		m.Cost.Cache.Read = cm.Cost.CacheRead
		m.Cost.Cache.Write = cm.Cost.CacheWrite
	}
	m.Capabilities.Temperature = cm.Temperature
	m.Capabilities.Reasoning = cm.Reasoning
	m.Capabilities.Attachment = cm.Attachment
	m.Capabilities.ToolCall = cm.ToolCall
	if cm.Modalities != nil {
		m.Capabilities.Input = modalitiesToCaps(cm.Modalities.Input)
		m.Capabilities.Output = modalitiesToCaps(cm.Modalities.Output)
	}
}

func modalitiesToCaps(mods []string) ModalityCaps {
	var c ModalityCaps
	for _, m := range mods {
		switch strings.ToLower(m) {
		case "text":
			c.Text = true
		case "audio":
			c.Audio = true
		case "image":
			c.Image = true
		case "video":
			c.Video = true
		case "pdf":
			c.PDF = true
		}
	}
	return c
}

// ApplyRemoteCatalog is a fail-open helper for serve/TUI startup.
func ApplyRemoteCatalog(ctx context.Context, reg *Registry) {
	cat, err := LoadCatalog(ctx)
	if err != nil {
		slog.Debug("model catalog unavailable", "error", err)
		return
	}
	if cat == nil {
		return
	}
	remoteCatalog.Store(&cat)
	n := MergeCatalogIntoRegistry(reg, cat)
	if n > 0 {
		slog.Info("merged remote model catalog metadata", "models", n)
	}
}

func applyRemoteCatalogToProvider(info *Info) {
	ptr := remoteCatalog.Load()
	if ptr == nil || info == nil {
		return
	}
	cat := *ptr
	prov, ok := cat[info.ID]
	if !ok {
		return
	}
	for modelID, cm := range prov.Models {
		if modelID == "" {
			modelID = cm.ID
		}
		m, found := info.Models[modelID]
		if !found {
			compound := info.ID + "/" + modelID
			m, found = info.Models[compound]
			if !found {
				continue
			}
		}
		applyCatalogModel(m, cm)
	}
}
