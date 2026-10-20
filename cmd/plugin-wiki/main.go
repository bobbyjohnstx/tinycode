package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

const wikiDir = ".tinycode/wiki"

func writeTool() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "write",
		Description: "Create or update a wiki page. Pages persist across sessions in .tinycode/wiki/.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "Page title (used as filename slug)",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "Markdown body content (without frontmatter)",
				},
				"category": map[string]any{
					"type":        "string",
					"description": "Category: architecture, decision, pattern, convention, reference, troubleshooting (or any string)",
				},
				"tags": map[string]any{
					"type":        "string",
					"description": "Comma-separated tags",
				},
			},
			"required": []string{"title", "body"},
		},
		Execute: func(_ context.Context, args json.RawMessage, tc plugin.ToolContext) (string, error) {
			var input struct {
				Title    string `json:"title"`
				Body     string `json:"body"`
				Category string `json:"category"`
				Tags     string `json:"tags"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if strings.TrimSpace(input.Title) == "" {
				return "", fmt.Errorf("title is required")
			}
			if strings.TrimSpace(input.Body) == "" {
				return "", fmt.Errorf("body is required")
			}
			return doWrite(tc.Directory, input.Title, input.Body, input.Category, input.Tags)
		},
	}
}

func readTool() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "read",
		Description: "Read a wiki page by title or filename.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "Page title or filename (with or without .md)",
				},
			},
			"required": []string{"title"},
		},
		Execute: func(_ context.Context, args json.RawMessage, tc plugin.ToolContext) (string, error) {
			var input struct {
				Title string `json:"title"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if strings.TrimSpace(input.Title) == "" {
				return "", fmt.Errorf("title is required")
			}
			return doRead(tc.Directory, input.Title)
		},
	}
}

func searchTool() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "search",
		Description: "Search wiki pages by keyword in title, body, tags, or category.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search keyword (case-insensitive substring match)",
				},
			},
			"required": []string{"query"},
		},
		Execute: func(_ context.Context, args json.RawMessage, tc plugin.ToolContext) (string, error) {
			var input struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return "", fmt.Errorf("parsing args: %w", err)
			}
			if strings.TrimSpace(input.Query) == "" {
				return "", fmt.Errorf("query is required")
			}
			return doSearch(tc.Directory, input.Query)
		},
	}
}

func listTool() plugin.ToolDef {
	return plugin.ToolDef{
		Name:        "list",
		Description: "List all wiki pages with title, category, tags, and last updated.",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(_ context.Context, _ json.RawMessage, tc plugin.ToolContext) (string, error) {
			return doList(tc.Directory)
		},
	}
}

// --- implementation ---

func wikiPath(projectDir string) string {
	return filepath.Join(projectDir, wikiDir)
}

func doWrite(projectDir, title, body, category, tags string) (string, error) {
	dir := wikiPath(projectDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create wiki dir: %w", err)
	}

	slug := slugify(title)
	path := filepath.Join(dir, slug+".md")

	now := time.Now().UTC().Format(time.RFC3339)
	created := now

	if existing, err := os.ReadFile(path); err == nil {
		if fm, _ := parseFrontmatter(string(existing)); fm != nil {
			if c, ok := fm["created"]; ok {
				created = c
			}
		}
	}

	if category == "" {
		category = "reference"
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "title: %s\n", title)
	fmt.Fprintf(&sb, "category: %s\n", category)
	if tags != "" {
		fmt.Fprintf(&sb, "tags: %s\n", tags)
	}
	fmt.Fprintf(&sb, "created: %s\n", created)
	fmt.Fprintf(&sb, "updated: %s\n", now)
	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimSpace(body))
	sb.WriteString("\n")

	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return "", fmt.Errorf("write wiki page: %w", err)
	}

	return fmt.Sprintf("Wiki page written: %s (%s)", title, path), nil
}

func doRead(projectDir, title string) (string, error) {
	path, err := resolvePage(projectDir, title)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read wiki page: %w", err)
	}
	return string(data), nil
}

func doSearch(projectDir, query string) (string, error) {
	dir := wikiPath(projectDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "No wiki pages found.", nil
		}
		return "", fmt.Errorf("read wiki dir: %w", err)
	}

	q := strings.ToLower(query)
	var matches []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		content := string(data)
		if strings.Contains(strings.ToLower(content), q) {
			fm, _ := parseFrontmatter(content)
			title := fmStr(fm, "title")
			if title == "" {
				title = strings.TrimSuffix(e.Name(), ".md")
			}
			cat := fmStr(fm, "category")
			tags := fmStr(fm, "tags")
			line := fmt.Sprintf("- %s", title)
			if cat != "" {
				line += fmt.Sprintf(" [%s]", cat)
			}
			if tags != "" {
				line += fmt.Sprintf(" (%s)", tags)
			}
			matches = append(matches, line)
		}
	}

	if len(matches) == 0 {
		return fmt.Sprintf("No wiki pages match %q.", query), nil
	}
	return fmt.Sprintf("Found %d page(s):\n%s", len(matches), strings.Join(matches, "\n")), nil
}

func doList(projectDir string) (string, error) {
	dir := wikiPath(projectDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "No wiki pages. Use wiki_write to create one.", nil
		}
		return "", fmt.Errorf("read wiki dir: %w", err)
	}

	type pageInfo struct {
		title    string
		category string
		tags     string
		updated  string
	}
	var pages []pageInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		fm, _ := parseFrontmatter(string(data))
		title := fmStr(fm, "title")
		if title == "" {
			title = strings.TrimSuffix(e.Name(), ".md")
		}
		pages = append(pages, pageInfo{
			title:    title,
			category: fmStr(fm, "category"),
			tags:     fmStr(fm, "tags"),
			updated:  fmStr(fm, "updated"),
		})
	}

	if len(pages) == 0 {
		return "No wiki pages. Use wiki_write to create one.", nil
	}

	sort.Slice(pages, func(i, j int) bool {
		return pages[i].updated > pages[j].updated
	})

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d wiki page(s):\n", len(pages))
	for _, p := range pages {
		sb.WriteString("- ")
		sb.WriteString(p.title)
		if p.category != "" {
			fmt.Fprintf(&sb, " [%s]", p.category)
		}
		if p.tags != "" {
			fmt.Fprintf(&sb, " (%s)", p.tags)
		}
		if p.updated != "" {
			fmt.Fprintf(&sb, " — %s", p.updated)
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// resolvePage finds a wiki page by title match or filename.
func resolvePage(projectDir, title string) (string, error) {
	dir := wikiPath(projectDir)

	// Try exact slug match first.
	slug := slugify(title)
	path := filepath.Join(dir, slug+".md")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	// Try with .md suffix if provided without.
	if !strings.HasSuffix(title, ".md") {
		path = filepath.Join(dir, title+".md")
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	} else {
		path = filepath.Join(dir, title)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Scan for title match in frontmatter.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("wiki page %q not found", title)
	}
	lower := strings.ToLower(title)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		fm, _ := parseFrontmatter(string(data))
		if strings.ToLower(fmStr(fm, "title")) == lower {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("wiki page %q not found", title)
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = nonAlphaNum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "untitled"
	}
	return s
}

// parseFrontmatter extracts YAML-like frontmatter from markdown.
func parseFrontmatter(content string) (map[string]string, string) {
	if !strings.HasPrefix(content, "---\n") {
		return nil, content
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return nil, content
	}
	fm := content[4 : 4+end]
	body := content[4+end+4:]

	result := make(map[string]string)
	for _, line := range strings.Split(fm, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result, body
}

func fmStr(fm map[string]string, key string) string {
	if fm == nil {
		return ""
	}
	return fm[key]
}

func main() {
	plugin.Run(plugin.Plugin{
		ID: "wiki",
		Tools: []plugin.ToolDef{
			writeTool(),
			readTool(),
			searchTool(),
			listTool(),
		},
	})
}
