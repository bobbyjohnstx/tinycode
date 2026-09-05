package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const warmupTimeout = 30 * time.Second

// WarmupProbe sends a minimal chat completion request with a tool definition
// to verify that the model supports tool calling. Returns true if the model
// responded with a valid tool call, false otherwise.
func WarmupProbe(ctx context.Context, baseURL, modelID string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, warmupTimeout)
	defer cancel()

	payload, _ := json.Marshal(warmupRequest(modelID))

	url := strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return false, fmt.Errorf("creating warmup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("warmup probe: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("warmup probe returned %d", resp.StatusCode)
	}

	return parseWarmupResponse(resp)
}

func warmupRequest(modelID string) map[string]any {
	return map[string]any{
		"model": modelID,
		"messages": []map[string]string{
			{"role": "user", "content": "What is 2+2? Use the calculator tool."},
		},
		"tools": []map[string]any{
			{
				"type": "function",
				"function": map[string]any{
					"name":        "calculator",
					"description": "Perform arithmetic",
					"parameters": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"expression": map[string]string{
								"type":        "string",
								"description": "Math expression to evaluate",
							},
						},
						"required": []string{"expression"},
					},
				},
			},
		},
		"max_tokens": 256,
	}
}

func parseWarmupResponse(resp *http.Response) (bool, error) {
	var body struct {
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					Function struct {
						Name string `json:"name"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, fmt.Errorf("parsing warmup response: %w", err)
	}
	for _, choice := range body.Choices {
		if len(choice.Message.ToolCalls) > 0 {
			return true, nil
		}
	}
	return false, nil
}
