package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

const ddgHTMLURL = "https://html.duckduckgo.com/html/"
const userAgent = "tinycode-plugin-gen-web-search/0.1.0"
const defaultMaxResults = 8

// searchResult holds a single search result.
type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

var (
	resultAPattern   = regexp.MustCompile(`<a\s[^>]*class="result__a"[^>]*href="([^"]*)"[^>]*>([\s\S]*?)</a>`)
	snippetPattern   = regexp.MustCompile(`<a\s[^>]*class="result__snippet"[^>]*>([\s\S]*?)</a>`)
	htmlTagPattern   = regexp.MustCompile(`<[^>]*>`)
	htmlEntityDecode = strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&apos;", "'",
	)
)

// stripHTML removes HTML tags and decodes common entities.
func stripHTML(s string) string {
	s = htmlTagPattern.ReplaceAllString(s, "")
	s = htmlEntityDecode.Replace(s)
	return strings.TrimSpace(s)
}

// parseDDGHTML parses DuckDuckGo HTML search results.
func parseDDGHTML(html string) []searchResult {
	titleMatches := resultAPattern.FindAllStringSubmatch(html, -1)
	snippetMatches := snippetPattern.FindAllStringSubmatch(html, -1)

	var results []searchResult
	for i, m := range titleMatches {
		r := searchResult{
			URL:   m[1],
			Title: stripHTML(m[2]),
		}
		if i < len(snippetMatches) {
			r.Snippet = stripHTML(snippetMatches[i][1])
		}
		results = append(results, r)
	}
	return results
}

// formatResults formats search results as a numbered markdown list.
func formatResults(results []searchResult) string {
	if len(results) == 0 {
		return "No results found."
	}
	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "%d. **%s** - %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet)
	}
	return sb.String()
}

// doSearch performs a web search via DuckDuckGo HTML endpoint.
func doSearch(ctx context.Context, client *http.Client, query string, maxResults int) (string, error) {
	form := url.Values{"q": {query}}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ddgHTMLURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("Search unavailable: HTTP %d", resp.StatusCode), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	results := parseDDGHTML(string(body))
	if maxResults > 0 && len(results) > maxResults {
		results = results[:maxResults]
	}
	return formatResults(results), nil
}

// searchArgs is the input schema for web_search and rh_kb_search.
type searchArgs struct {
	Query      string `json:"query"`
	MaxResults *int   `json:"maxResults,omitempty"`
}

// newPlugin returns the web-search plugin definition.
func newPlugin(client *http.Client) plugin.Plugin {
	return plugin.Plugin{
		ID: "web-search",
		Tools: []plugin.ToolDef{
			{
				Name:        "web_search",
				Description: "Search the web using DuckDuckGo. Returns titles, URLs, and snippets for matching pages.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query":      map[string]any{"type": "string", "description": "Search query"},
						"maxResults": map[string]any{"type": "integer", "description": "Maximum number of results to return (default 8)"},
					},
					"required": []string{"query"},
				},
				Execute: func(ctx context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
					var args searchArgs
					if err := json.Unmarshal(raw, &args); err != nil {
						return "", fmt.Errorf("invalid arguments: %w", err)
					}
					if args.Query == "" {
						return "", fmt.Errorf("query is required")
					}
					max := defaultMaxResults
					if args.MaxResults != nil {
						max = *args.MaxResults
					}
					result, err := doSearch(ctx, client, args.Query, max)
					if err != nil {
						return fmt.Sprintf("Search unavailable: %v", err), nil
					}
					return result, nil
				},
			},
			{
				Name:        "rh_kb_search",
				Description: "Search the Red Hat knowledge base (access.redhat.com) using DuckDuckGo. Returns titles, URLs, and snippets for matching KB articles.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query":      map[string]any{"type": "string", "description": "Search query for Red Hat knowledge base"},
						"maxResults": map[string]any{"type": "integer", "description": "Maximum number of results to return (default 8)"},
					},
					"required": []string{"query"},
				},
				Execute: func(ctx context.Context, raw json.RawMessage, _ plugin.ToolContext) (string, error) {
					var args searchArgs
					if err := json.Unmarshal(raw, &args); err != nil {
						return "", fmt.Errorf("invalid arguments: %w", err)
					}
					if args.Query == "" {
						return "", fmt.Errorf("query is required")
					}
					max := defaultMaxResults
					if args.MaxResults != nil {
						max = *args.MaxResults
					}
					scopedQuery := "site:access.redhat.com " + args.Query
					result, err := doSearch(ctx, client, scopedQuery, max)
					if err != nil {
						return fmt.Sprintf("Search unavailable: %v", err), nil
					}
					return result, nil
				},
			},
		},
	}
}

func main() {
	plugin.Run(newPlugin(http.DefaultClient))
}
