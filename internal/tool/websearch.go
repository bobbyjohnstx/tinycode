package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	exaAPIURL          = "https://api.exa.ai/search"
	exaDefaultResults  = 8
	exaRequestTimeout  = 30 * time.Second
)

type webSearchArgs struct {
	Query      string `json:"query"`
	NumResults *int   `json:"num_results,omitempty"`
}

type exaRequest struct {
	Query      string `json:"query"`
	NumResults int    `json:"numResults"`
}

type exaResponse struct {
	Results []exaResult `json:"results"`
}

type exaResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text,omitempty"`
}

// WebSearchTool returns a tool that searches the web using the Exa API.
// Only register when EXA_API_KEY is set.
func WebSearchTool() *Def {
	return &Def{
		ID:          "websearch",
		Description: "Search the web using the Exa API and return results as markdown.",
		Permission:  "read",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query",
				},
				"num_results": map[string]any{
					"type":        "integer",
					"description": "Number of results to return (default: 8)",
				},
			},
			"required": []string{"query"},
		},
		Execute: executeWebSearch,
	}
}

func executeWebSearch(ctx context.Context, _ *Context, rawArgs json.RawMessage) (*ExecuteResult, error) {
	var args webSearchArgs
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Invalid arguments: %v", err), IsError: true}, nil
	}

	if args.Query == "" {
		return &ExecuteResult{Output: "query is required", IsError: true}, nil
	}

	apiKey := os.Getenv("EXA_API_KEY")
	if apiKey == "" {
		return &ExecuteResult{Output: "EXA_API_KEY environment variable is not set", IsError: true}, nil
	}

	numResults := exaDefaultResults
	if args.NumResults != nil && *args.NumResults > 0 {
		numResults = *args.NumResults
	}

	reqBody := exaRequest{
		Query:      args.Query,
		NumResults: numResults,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Failed to marshal request: %v", err), IsError: true}, nil
	}

	reqCtx, cancel := context.WithTimeout(ctx, exaRequestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(reqCtx, "POST", exaAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Failed to create request: %v", err), IsError: true}, nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)

	resp, err := fetchClient.Do(httpReq)
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Search request failed: %v", err), IsError: true}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize))
	if err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Failed to read response: %v", err), IsError: true}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &ExecuteResult{
			Output:  fmt.Sprintf("Exa API error (HTTP %d): %s", resp.StatusCode, string(respBody)),
			IsError: true,
		}, nil
	}

	var exaResp exaResponse
	if err := json.Unmarshal(respBody, &exaResp); err != nil {
		return &ExecuteResult{Output: fmt.Sprintf("Failed to parse response: %v", err), IsError: true}, nil
	}

	return &ExecuteResult{Output: formatExaResults(args.Query, exaResp.Results)}, nil
}

func formatExaResults(query string, results []exaResult) string {
	if len(results) == 0 {
		return fmt.Sprintf("No results found for: %s", query)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Search results for: %s\n\n", query))
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("### %d. %s\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("**URL:** %s\n", r.URL))
		if r.Text != "" {
			sb.WriteString(fmt.Sprintf("\n%s\n", r.Text))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
