package render

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
)

// MarkdownRenderer renders markdown to styled terminal output using glamour.
type MarkdownRenderer struct {
	renderer *glamour.TermRenderer
	width    int
}

// glamourStyleName returns the style name we will apply. Empty/auto → "dark".
func glamourStyleName() string {
	style := os.Getenv("GLAMOUR_STYLE")
	if style == "" || style == "auto" {
		return "dark"
	}
	return style
}

func glamourStyleOption() glamour.TermRendererOption {
	name := glamourStyleName()
	// API caveat: never call WithEnvironmentConfig() alone — empty env becomes
	// styles.AutoStyle and still probes the terminal (OSC 11 + CPR).
	// Never pass "auto" to WithStandardStyle/WithStylePath either (same probe).
	if name == "dark" || name == "light" || name == "notty" || name == "ascii" || name == "pink" {
		return glamour.WithStandardStyle(name)
	}
	// Custom JSON path or other glamour-recognized name.
	return glamour.WithStylePath(name)
}

// NewMarkdownRenderer creates a renderer for the given terminal width.
func NewMarkdownRenderer(width int) *MarkdownRenderer {
	if width < 20 {
		width = 20
	}
	r, err := glamour.NewTermRenderer(
		glamourStyleOption(),
		glamour.WithWordWrap(width),
	)
	if r == nil || err != nil {
		// Fallback: return a renderer that passes through raw text.
		return &MarkdownRenderer{width: width}
	}
	return &MarkdownRenderer{renderer: r, width: width}
}

// RenderFinal renders the complete markdown text through glamour.
func (m *MarkdownRenderer) RenderFinal(text string) string {
	if text == "" {
		return ""
	}
	if m.renderer == nil {
		return text
	}
	out, err := m.renderer.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimRight(out, "\n")
}

// RenderStreaming renders markdown that may still be arriving.
// Complete blocks (separated by double newlines) are rendered through glamour.
// The trailing incomplete block is returned as raw text.
func (m *MarkdownRenderer) RenderStreaming(text string) string {
	if text == "" {
		return ""
	}

	// Split on double newlines to find block boundaries.
	lastSep := strings.LastIndex(text, "\n\n")
	if lastSep < 0 {
		// No complete block yet, return raw text.
		return text
	}

	complete := text[:lastSep]
	trailing := text[lastSep+2:]

	var sb strings.Builder

	// Render complete blocks through glamour.
	rendered := m.RenderFinal(complete)
	sb.WriteString(rendered)

	// Append trailing incomplete text as-is.
	if trailing != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(trailing)
	}

	return sb.String()
}
