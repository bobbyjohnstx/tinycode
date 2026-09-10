package main

import (
	"strings"
	"testing"
)

func TestPluginID(t *testing.T) {
	p := newPlugin()
	if p.ID != "rh-dev-content" {
		t.Errorf("got %q, want %q", p.ID, "rh-dev-content")
	}
}

func TestToolDefinitions(t *testing.T) {
	p := newPlugin()
	if len(p.Tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(p.Tools))
	}
	wantNames := []string{"rh_dev_search", "rh_dev_article", "rh_dev_recent"}
	for i, want := range wantNames {
		if p.Tools[i].Name != want {
			t.Errorf("tool[%d] name = %q, want %q", i, p.Tools[i].Name, want)
		}
		if p.Tools[i].Execute == nil {
			t.Errorf("tool[%d] Execute is nil", i)
		}
	}
}

func TestToolSchemas(t *testing.T) {
	p := newPlugin()
	for _, tool := range p.Tools {
		params := tool.Parameters
		if params["type"] != "object" {
			t.Errorf("%s: params type = %v, want %q", tool.Name, params["type"], "object")
		}
		if _, ok := params["properties"].(map[string]any); !ok {
			t.Errorf("%s: properties is not map[string]any", tool.Name)
		}
	}
}

func TestIsValidTopic(t *testing.T) {
	tests := []struct {
		topic string
		want  bool
	}{
		{"kubernetes", true},
		{"python", true},
		{"ai-ml", true},
		{"containers", true},
		{"nonexistent-topic", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			if got := isValidTopic(tt.topic); got != tt.want {
				t.Errorf("isValidTopic(%q) = %v, want %v", tt.topic, got, tt.want)
			}
		})
	}
}

func TestParseTopicPage(t *testing.T) {
	html := `<div>
		<a href="/articles/2026/my-article">Article Title Here</a>
		<a href="/articles/2026/another-article">Another Good Article</a>
		<a href="/articles/2026/short">Hi</a>
	</div>`
	articles := parseTopicPage(html)
	if len(articles) != 2 {
		t.Fatalf("got %d articles, want 2 (short title filtered)", len(articles))
	}
	if articles[0].Title != "Article Title Here" {
		t.Errorf("first title = %q", articles[0].Title)
	}
	if !strings.HasSuffix(articles[0].URL, "/articles/2026/my-article") {
		t.Errorf("first URL = %q", articles[0].URL)
	}
}

func TestParseTopicPage_Dedup(t *testing.T) {
	html := `<div>
		<a href="/articles/2026/same-article">Article Title One</a>
		<a href="/articles/2026/same-article">Article Title Two</a>
	</div>`
	articles := parseTopicPage(html)
	if len(articles) != 1 {
		t.Errorf("got %d articles, want 1 (deduped)", len(articles))
	}
}

func TestParseRSSFeed(t *testing.T) {
	xml := `<?xml version="1.0"?>
<rss>
<channel>
<item>
<title><![CDATA[Test Article]]></title>
<link>https://developers.redhat.com/blog/2026/test</link>
<pubDate>Mon, 01 Jan 2026 12:00:00 +0000</pubDate>
<dc:creator><![CDATA[Jane Doe]]></dc:creator>
</item>
<item>
<title>Plain Title</title>
<link>https://developers.redhat.com/blog/2026/plain</link>
</item>
</channel>
</rss>`
	articles := parseRSSFeed(xml)
	if len(articles) != 2 {
		t.Fatalf("got %d articles, want 2", len(articles))
	}
	if articles[0].Title != "Test Article" {
		t.Errorf("first title = %q", articles[0].Title)
	}
	if articles[0].Author != "Jane Doe" {
		t.Errorf("first author = %q", articles[0].Author)
	}
	if articles[0].Date != "2026-01-01" {
		t.Errorf("first date = %q", articles[0].Date)
	}
	if articles[1].Title != "Plain Title" {
		t.Errorf("second title = %q", articles[1].Title)
	}
}

func TestExtractArticleContent(t *testing.T) {
	t.Run("body div extraction", func(t *testing.T) {
		html := `<div class="field--name-body"><p>This is the article body.</p></div></div></div>`
		got := extractArticleContent(html)
		if !strings.Contains(got, "This is the article body") {
			t.Errorf("missing body content in: %q", got)
		}
	})

	t.Run("main tag fallback", func(t *testing.T) {
		html := `<main><p>Main content here.</p></main>`
		got := extractArticleContent(html)
		if !strings.Contains(got, "Main content here") {
			t.Errorf("missing main content in: %q", got)
		}
	})

	t.Run("article tag fallback", func(t *testing.T) {
		html := `<article><p>Article content.</p></article>`
		got := extractArticleContent(html)
		if !strings.Contains(got, "Article content") {
			t.Errorf("missing article content in: %q", got)
		}
	})
}

func TestExtractArticleMeta(t *testing.T) {
	html := `<html>
<head>
<title>My Article Title</title>
<meta name="author" content="John Smith">
<meta property="article:published_time" content="2026-03-15T00:00:00Z">
</head>
<body></body>
</html>`
	meta := extractArticleMeta(html)
	if meta.Title != "My Article Title" {
		t.Errorf("title = %q", meta.Title)
	}
	if meta.Author != "John Smith" {
		t.Errorf("author = %q", meta.Author)
	}
	if meta.Date != "2026-03-15" {
		t.Errorf("date = %q", meta.Date)
	}
}

func TestFormatArticleLink(t *testing.T) {
	link := articleLink{
		Title:  "Test Article",
		URL:    "https://example.com/test",
		Date:   "2026-01-01",
		Author: "Jane Doe",
	}
	got := formatArticleLink(link)
	for _, want := range []string{"Test Article", "https://example.com/test", "Date: 2026-01-01", "Author: Jane Doe"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in: %q", want, got)
		}
	}
}
