package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/redhat"
	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

const baseURL = "https://developers.redhat.com"

var topics = []string{
	"kubernetes", "containers", "kubernetes/operators",
	"automation", "devops", "devsecops", "ansible-automation-applications-and-services", "ci-cd",
	"enterprise-java", "python", "go", "rust", "nodejs", "dotnet",
	"gitops", "developer-productivity", "developer-tools",
	"observability", "microservices", "serverless", "event-driven", "api-management",
	"security", "secure-coding",
	"ai-ml", "data-science", "kafka-kubernetes",
	"linux", "virtualization", "edge-computing", "databases",
}

var httpClient = &http.Client{Timeout: 30 * time.Second}

func fetchHTML(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "tinycode-plugin-dev-content/0.1.0")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

type articleLink struct {
	Title  string
	URL    string
	Date   string
	Author string
}

var linkPattern = regexp.MustCompile(`(?is)<a[^>]*href="(/articles/[^"]+)"[^>]*>([\s\S]*?)</a>`)

func parseTopicPage(html string) []articleLink {
	matches := linkPattern.FindAllStringSubmatch(html, -1)
	seen := make(map[string]bool)
	var articles []articleLink
	for _, m := range matches {
		url := m[1]
		title := redhat.StripHTML(m[2])
		if len(title) > 10 && !seen[url] {
			seen[url] = true
			articles = append(articles, articleLink{
				Title: title,
				URL:   baseURL + url,
			})
		}
	}
	return articles
}

var (
	itemPattern  = regexp.MustCompile(`(?is)<item>([\s\S]*?)</item>`)
	titleCDATA   = regexp.MustCompile(`<title><!\[CDATA\[(.*?)\]\]></title>`)
	titlePlain   = regexp.MustCompile(`<title>(.*?)</title>`)
	linkTag      = regexp.MustCompile(`<link>(.*?)</link>`)
	pubDateTag   = regexp.MustCompile(`<pubDate>(.*?)</pubDate>`)
	creatorCDATA = regexp.MustCompile(`<dc:creator><!\[CDATA\[(.*?)\]\]></dc:creator>`)
	creatorPlain = regexp.MustCompile(`<dc:creator>(.*?)</dc:creator>`)
)

func parseRSSFeed(xml string) []articleLink {
	items := itemPattern.FindAllStringSubmatch(xml, -1)
	var articles []articleLink
	for _, m := range items {
		item := m[1]
		title := "Untitled"
		if sm := titleCDATA.FindStringSubmatch(item); sm != nil {
			title = sm[1]
		} else if sm := titlePlain.FindStringSubmatch(item); sm != nil {
			title = sm[1]
		}
		link := ""
		if sm := linkTag.FindStringSubmatch(item); sm != nil {
			link = sm[1]
		}
		date := ""
		if sm := pubDateTag.FindStringSubmatch(item); sm != nil {
			if t, err := time.Parse(time.RFC1123Z, sm[1]); err == nil {
				date = t.Format("2006-01-02")
			} else if t, err := time.Parse(time.RFC1123, sm[1]); err == nil {
				date = t.Format("2006-01-02")
			}
		}
		author := ""
		if sm := creatorCDATA.FindStringSubmatch(item); sm != nil {
			author = sm[1]
		} else if sm := creatorPlain.FindStringSubmatch(item); sm != nil {
			author = sm[1]
		}
		if link != "" {
			articles = append(articles, articleLink{
				Title:  redhat.StripHTML(title),
				URL:    link,
				Date:   date,
				Author: author,
			})
		}
	}
	return articles
}

var (
	bodyDivRe    = regexp.MustCompile(`(?is)<div[^>]*class="[^"]*field--name-body[^"]*"[^>]*>([\s\S]*?)</div>\s*</div>\s*</div>`)
	mainTagRe    = regexp.MustCompile(`(?is)<main[^>]*>([\s\S]*?)</main>`)
	articleTagRe = regexp.MustCompile(`(?is)<article[^>]*>([\s\S]*?)</article>`)
)

func extractArticleContent(html string) string {
	if m := bodyDivRe.FindStringSubmatch(html); m != nil {
		return redhat.StripHTML(m[1])
	}
	if m := mainTagRe.FindStringSubmatch(html); m != nil {
		return redhat.StripHTML(m[1])
	}
	if m := articleTagRe.FindStringSubmatch(html); m != nil {
		return redhat.StripHTML(m[1])
	}
	s := redhat.StripHTML(html)
	if len(s) > 5000 {
		return s[:5000]
	}
	return s
}

var (
	htmlTitleRe    = regexp.MustCompile(`(?i)<title>(.*?)</title>`)
	h1Re           = regexp.MustCompile(`(?i)<h1[^>]*>(.*?)</h1>`)
	metaAuthorRe   = regexp.MustCompile(`(?i)<meta[^>]*name="author"[^>]*content="([^"]+)"`)
	metaDateRe     = regexp.MustCompile(`(?i)<meta[^>]*property="article:published_time"[^>]*content="([^"]+)"`)
	timeDatetimeRe = regexp.MustCompile(`(?i)<time[^>]*datetime="([^"]+)"`)
)

type articleMeta struct {
	Title  string
	Author string
	Date   string
}

func extractArticleMeta(html string) articleMeta {
	meta := articleMeta{Title: "Untitled"}
	if m := htmlTitleRe.FindStringSubmatch(html); m != nil {
		meta.Title = redhat.StripHTML(m[1])
	} else if m := h1Re.FindStringSubmatch(html); m != nil {
		meta.Title = redhat.StripHTML(m[1])
	}
	if m := metaAuthorRe.FindStringSubmatch(html); m != nil {
		meta.Author = m[1]
	}
	if m := metaDateRe.FindStringSubmatch(html); m != nil {
		meta.Date = strings.SplitN(m[1], "T", 2)[0]
	} else if m := timeDatetimeRe.FindStringSubmatch(html); m != nil {
		meta.Date = strings.SplitN(m[1], "T", 2)[0]
	}
	return meta
}

func formatArticleLink(a articleLink) string {
	parts := []string{a.Title, a.URL}
	if a.Date != "" {
		parts = append(parts, "Date: "+a.Date)
	}
	if a.Author != "" {
		parts = append(parts, "Author: "+a.Author)
	}
	return strings.Join(parts, " | ")
}

func isValidTopic(topic string) bool {
	for _, t := range topics {
		if t == topic {
			return true
		}
	}
	return false
}

func buildTools() []plugin.ToolDef {
	return []plugin.ToolDef{
		{
			Name:        "rh_dev_search",
			Description: "Browse Red Hat developer articles by topic. Available topics include: kubernetes, containers, ai-ml, python, go, rust, nodejs, enterprise-java, security, devops, gitops, automation, microservices, and more.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"topic": map[string]any{"type": "string", "description": "Topic slug (e.g., kubernetes, ai-ml, python, containers, security, devops, gitops, enterprise-java)"},
					"page":  map[string]any{"type": "integer", "description": "Page number (default 1, 25 articles per page)"},
				},
				"required": []string{"topic"},
			},
			Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Topic string `json:"topic"`
					Page  int    `json:"page"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				topic := strings.ToLower(input.Topic)
				if !isValidTopic(topic) {
					return fmt.Sprintf("Unknown topic %q. Available topics: %s", topic, strings.Join(topics, ", ")), nil
				}
				page := input.Page
				if page <= 0 {
					page = 1
				}
				url := fmt.Sprintf("%s/topics/%s/all?page=%d", baseURL, topic, page)
				html, err := fetchHTML(url)
				if err != nil {
					return fmt.Sprintf("Search failed: %v", err), nil
				}
				articles := parseTopicPage(html)
				if len(articles) == 0 {
					return fmt.Sprintf("No articles found for topic %q on page %d.", topic, page), nil
				}
				var lines []string
				for _, a := range articles {
					lines = append(lines, formatArticleLink(a))
				}
				return fmt.Sprintf("Articles on %q (page %d):\n%s", topic, page, strings.Join(lines, "\n")), nil
			},
		},
		{
			Name:        "rh_dev_article",
			Description: "Read the full content of a Red Hat developer article by URL. Returns the article title, metadata, and body text.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{"type": "string", "description": "Full URL of the article (e.g., https://developers.redhat.com/articles/2026/...)"},
				},
				"required": []string{"url"},
			},
			Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					URL string `json:"url"`
				}
				if err := json.Unmarshal(args, &input); err != nil {
					return "", fmt.Errorf("parsing args: %w", err)
				}
				html, err := fetchHTML(input.URL)
				if err != nil {
					return fmt.Sprintf("Failed to read article: %v", err), nil
				}
				meta := extractArticleMeta(html)
				content := extractArticleContent(html)

				var header []string
				header = append(header, "# "+meta.Title)
				if meta.Author != "" {
					header = append(header, "Author: "+meta.Author)
				}
				if meta.Date != "" {
					header = append(header, "Date: "+meta.Date)
				}
				header = append(header, "")

				truncated := content
				if len(truncated) > 8000 {
					truncated = truncated[:8000] + "\n\n[Content truncated]"
				}
				return strings.Join(header, "\n") + truncated, nil
			},
		},
		{
			Name:        "rh_dev_recent",
			Description: "Get the most recent Red Hat developer articles from the RSS feed. Returns titles, URLs, dates, and authors.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{"type": "integer", "description": "Number of articles to return (default 10, max 25)"},
				},
			},
			Execute: func(_ context.Context, args json.RawMessage, _ plugin.ToolContext) (string, error) {
				var input struct {
					Limit int `json:"limit"`
				}
				if err := plugin.UnmarshalToolArgs(args, &input); err != nil {
					return "", err
				}
				xml, err := fetchHTML(baseURL + "/blog/feed/")
				if err != nil {
					return fmt.Sprintf("Failed to fetch recent articles: %v", err), nil
				}
				articles := parseRSSFeed(xml)
				limit := input.Limit
				if limit <= 0 {
					limit = 10
				}
				limit = int(math.Min(float64(limit), 25))
				if limit > len(articles) {
					limit = len(articles)
				}
				limited := articles[:limit]
				if len(limited) == 0 {
					return "No recent articles found.", nil
				}
				var lines []string
				for _, a := range limited {
					lines = append(lines, formatArticleLink(a))
				}
				return "Recent Red Hat developer articles:\n" + strings.Join(lines, "\n"), nil
			},
		},
	}
}

func newPlugin() plugin.Plugin {
	return plugin.Plugin{
		ID:    "rh-dev-content",
		Tools: buildTools(),
	}
}

func main() {
	plugin.Run(newPlugin())
}
