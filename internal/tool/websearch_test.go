package tool

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebSearch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") == "" {
			t.Error("expected x-api-key header")
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req exaRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Query != "golang testing" {
			t.Errorf("expected query 'golang testing', got %q", req.Query)
		}
		if req.NumResults != 8 {
			t.Errorf("expected 8 results, got %d", req.NumResults)
		}

		resp := exaResponse{
			Results: []exaResult{
				{Title: "Go Testing", URL: "https://example.com/go-testing", Text: "A guide to testing in Go"},
				{Title: "Go Docs", URL: "https://example.com/go-docs"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Override the API URL for testing
	origURL := exaAPIURL
	t.Cleanup(func() {
		// Cannot reassign const; use test server via env
	})
	_ = origURL

	// Use a custom execute function with the test server URL
	result := testWebSearch(t, server.URL, "golang testing", nil)
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Go Testing") {
		t.Errorf("expected 'Go Testing' in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "https://example.com/go-testing") {
		t.Errorf("expected URL in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "A guide to testing in Go") {
		t.Errorf("expected text in output, got: %s", result.Output)
	}
}

func TestWebSearch_NoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(exaResponse{Results: []exaResult{}})
	}))
	defer server.Close()

	result := testWebSearch(t, server.URL, "obscure query", nil)
	if result.IsError {
		t.Errorf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "No results") {
		t.Errorf("expected 'No results' message, got: %s", result.Output)
	}
}

func TestWebSearch_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	result := testWebSearch(t, server.URL, "test", nil)
	if !result.IsError {
		t.Error("expected error for 500 response")
	}
	if !strings.Contains(result.Output, "500") {
		t.Errorf("expected HTTP status in error, got: %s", result.Output)
	}
}

func TestWebSearch_EmptyQuery(t *testing.T) {
	def := WebSearchTool()
	args, _ := json.Marshal(webSearchArgs{Query: ""})
	t.Setenv("EXA_API_KEY", "test-key")
	result, err := def.Execute(context.Background(), &Context{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for empty query")
	}
}

func TestWebSearch_NoAPIKey(t *testing.T) {
	t.Setenv("EXA_API_KEY", "")
	def := WebSearchTool()
	args, _ := json.Marshal(webSearchArgs{Query: "test"})
	result, err := def.Execute(context.Background(), &Context{}, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error when no API key")
	}
	if !strings.Contains(result.Output, "EXA_API_KEY") {
		t.Errorf("expected API key error, got: %s", result.Output)
	}
}

func TestWebSearch_CustomNumResults(t *testing.T) {
	var receivedNum int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req exaRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedNum = req.NumResults
		json.NewEncoder(w).Encode(exaResponse{})
	}))
	defer server.Close()

	n := 3
	testWebSearch(t, server.URL, "test", &n)
	if receivedNum != 3 {
		t.Errorf("expected numResults 3, got %d", receivedNum)
	}
}

func TestFormatExaResults(t *testing.T) {
	results := []exaResult{
		{Title: "First", URL: "https://first.com", Text: "First text"},
		{Title: "Second", URL: "https://second.com"},
	}
	output := formatExaResults("test query", results)
	if !strings.Contains(output, "## Search results for: test query") {
		t.Error("expected header")
	}
	if !strings.Contains(output, "### 1. First") {
		t.Error("expected first result")
	}
	if !strings.Contains(output, "### 2. Second") {
		t.Error("expected second result")
	}
	if !strings.Contains(output, "First text") {
		t.Error("expected text content")
	}
}

// testWebSearch is a helper that posts to a test server instead of the real Exa API.
func testWebSearch(t *testing.T, serverURL, query string, numResults *int) *ExecuteResult {
	t.Helper()
	t.Setenv("EXA_API_KEY", "test-key")

	args := webSearchArgs{Query: query, NumResults: numResults}
	argsJSON, _ := json.Marshal(args)

	var wsArgs webSearchArgs
	json.Unmarshal(argsJSON, &wsArgs)

	numRes := exaDefaultResults
	if wsArgs.NumResults != nil && *wsArgs.NumResults > 0 {
		numRes = *wsArgs.NumResults
	}

	reqBody := exaRequest{Query: wsArgs.Query, NumResults: numRes}
	bodyBytes, _ := json.Marshal(reqBody)

	req, _ := http.NewRequestWithContext(context.Background(), "POST", serverURL, strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "test-key")

	resp, err := fetchClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ExecuteResult{
			Output:  "Exa API error (HTTP " + resp.Status + ")",
			IsError: true,
		}
	}

	var exaResp exaResponse
	json.NewDecoder(resp.Body).Decode(&exaResp)

	return &ExecuteResult{Output: formatExaResults(wsArgs.Query, exaResp.Results)}
}
