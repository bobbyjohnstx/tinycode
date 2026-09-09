package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var profileSuffixRe = regexp.MustCompile(`-tc\d+k$`)

func ProfileName(baseName string, numCtx int) string {
	return fmt.Sprintf("%s-tc%dk", baseName, int(math.Round(float64(numCtx)/1024)))
}

func IsProfile(modelName string) bool {
	return profileSuffixRe.MatchString(modelName)
}

func BaseModelName(name string) string {
	return profileSuffixRe.ReplaceAllString(name, "")
}

func NumCtxFromProfileName(name string) int {
	match := profileSuffixRe.FindString(name)
	if match == "" {
		return 0
	}
	numStr := match[3 : len(match)-1]
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return 0
	}
	return n * 1024
}

type OllamaShowResult struct {
	ParameterSize    string
	QuantizationLevel string
	BlockCount       int
	EmbeddingLength  int
	HeadCount        int
	HeadCountKV      int
	ContextLength    int
}

type AutoProfileConfig struct {
	Enabled       *bool                         `json:"enabled,omitempty"`
	DefaultNumCtx *int                          `json:"default_num_ctx,omitempty"`
	MaxNumCtx     *int                          `json:"max_num_ctx,omitempty"`
	Models        map[string]AutoProfileModel   `json:"models,omitempty"`
}

type AutoProfileModel struct {
	NumCtx *int `json:"num_ctx,omitempty"`
	Skip   bool `json:"skip,omitempty"`
}

var quantBPP = map[string]float64{
	"Q4_0":  0.5,
	"Q4_K_S": 0.53,
	"Q4_K_M": 0.55,
	"Q5_0":  0.625,
	"Q5_K_S": 0.63,
	"Q5_K_M": 0.65,
	"Q6_K":  0.75,
	"Q8_0":  1.0,
	"FP16":  2.0,
	"F16":   2.0,
	"BF16":  2.0,
}

const (
	defaultBPP    = 0.6
	minKVBudget   = 100 * 1024 * 1024
	minNumCtx     = 2048
	maxNumCtx     = 131072
)

// GPUMemoryBudget returns 50% of GPU memory, capped at 32GB.
func GPUMemoryBudget(gpuMemoryBytes int64) int64 {
	budget := gpuMemoryBytes / 2
	cap := int64(32) * 1024 * 1024 * 1024
	if budget > cap {
		return cap
	}
	return budget
}

// CalculateNumCtx computes the optimal num_ctx for a model based on available GPU memory.
func CalculateNumCtx(gpuMemoryBytes int64, info OllamaShowResult, advertisedCtx int) int {
	budget := GPUMemoryBudget(gpuMemoryBytes)

	paramSize := parseParameterSize(info.ParameterSize)
	bpp := quantBPP[info.QuantizationLevel]
	if bpp == 0 {
		bpp = defaultBPP
	}
	modelWeightBytes := int64(paramSize * bpp)
	kvBudgetBytes := budget - modelWeightBytes
	if kvBudgetBytes < minKVBudget {
		kvBudgetBytes = minKVBudget
	}

	if info.HeadCount <= 0 || info.BlockCount <= 0 || info.EmbeddingLength <= 0 {
		if advertisedCtx < 8192 {
			return advertisedCtx
		}
		return 8192
	}

	headDim := info.EmbeddingLength / info.HeadCount
	kvBytesPerToken := 2 * info.BlockCount * headDim * info.HeadCountKV * 2
	numCtx := int(kvBudgetBytes) / kvBytesPerToken

	numCtx = (numCtx / 1024) * 1024

	if numCtx < minNumCtx {
		numCtx = minNumCtx
	}
	if numCtx > advertisedCtx {
		numCtx = advertisedCtx
	}
	if numCtx > maxNumCtx {
		numCtx = maxNumCtx
	}

	return numCtx
}

var paramSizeRe = regexp.MustCompile(`(?i)([\d.]+)\s*[bB]`)

func parseParameterSize(s string) float64 {
	match := paramSizeRe.FindStringSubmatch(s)
	if match == nil {
		return 0
	}
	v, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return 0
	}
	return v * 1e9
}

const (
	ollamaAPITimeout    = 5 * time.Second
	ollamaCreateTimeout = 30 * time.Second
)

// ShowModel queries the Ollama API for model metadata.
func ShowModel(ctx context.Context, client *http.Client, baseURL, model string) (*OllamaShowResult, error) {
	ctx, cancel := context.WithTimeout(ctx, ollamaAPITimeout)
	defer cancel()

	if client == nil {
		client = http.DefaultClient
	}

	payload, _ := json.Marshal(map[string]string{"name": model})
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(baseURL, "/")+"/api/show", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama show returned %d", resp.StatusCode)
	}

	var body struct {
		Details struct {
			ParameterSize    string `json:"parameter_size"`
			QuantizationLevel string `json:"quantization_level"`
		} `json:"details"`
		ModelInfo map[string]any `json:"model_info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	if body.ModelInfo == nil {
		return nil, fmt.Errorf("no model_info in response")
	}

	return &OllamaShowResult{
		ParameterSize:    body.Details.ParameterSize,
		QuantizationLevel: body.Details.QuantizationLevel,
		BlockCount:       findModelInfoInt(body.ModelInfo, "block_count"),
		EmbeddingLength:  findModelInfoInt(body.ModelInfo, "embedding_length"),
		HeadCount:        findModelInfoInt(body.ModelInfo, "head_count"),
		HeadCountKV:      findModelInfoInt(body.ModelInfo, "head_count_kv"),
		ContextLength:    findModelInfoInt(body.ModelInfo, "context_length"),
	}, nil
}

func findModelInfoInt(info map[string]any, suffix string) int {
	for key, value := range info {
		if key == suffix || strings.HasSuffix(key, "."+suffix) {
			switch v := value.(type) {
			case float64:
				return int(v)
			case int:
				return v
			}
		}
	}
	return 0
}

// CreateProfile creates a derived Ollama model with a baked-in num_ctx.
func CreateProfile(ctx context.Context, client *http.Client, baseURL, baseName, profName string, numCtx int) error {
	ctx, cancel := context.WithTimeout(ctx, ollamaCreateTimeout)
	defer cancel()

	if client == nil {
		client = http.DefaultClient
	}

	payload, _ := json.Marshal(map[string]any{
		"model":      profName,
		"from":       baseName,
		"parameters": map[string]int{"num_ctx": numCtx},
	})

	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(baseURL, "/")+"/api/create", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama create returned %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"success"`) {
		return fmt.Errorf("profile creation did not succeed")
	}

	return nil
}

// DeleteModel removes a model from Ollama via the API.
func DeleteModel(ctx context.Context, client *http.Client, baseURL, model string) error {
	ctx, cancel := context.WithTimeout(ctx, ollamaAPITimeout)
	defer cancel()

	if client == nil {
		client = http.DefaultClient
	}

	payload, _ := json.Marshal(map[string]string{"model": model})
	req, err := http.NewRequestWithContext(ctx, "DELETE", strings.TrimRight(baseURL, "/")+"/api/delete", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama delete returned %d", resp.StatusCode)
	}
	return nil
}
