package render

import (
	"os"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
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

// MarkdownThemeColors carries resolved hex color strings for glamour styling.
// Empty strings mean "use the default glamour dark style value".
type MarkdownThemeColors struct {
	Text       string
	Heading    string
	Code       string
	Link       string
	BlockQuote string
}

// HasColors reports whether any theme color is set.
func (c MarkdownThemeColors) HasColors() bool {
	return c.Text != "" || c.Heading != "" || c.Code != "" || c.Link != "" || c.BlockQuote != ""
}

// MarkdownPresets maps named styles to hardcoded color values.
var MarkdownPresets = map[string]MarkdownThemeColors{
	"dark": {
		Text:       "#e0e0e0",
		Heading:    "#61afef",
		Code:       "#98c379",
		Link:       "#56b6c2",
		BlockQuote: "#abb2bf",
	},
	"light": {
		Text:       "#383a42",
		Heading:    "#4078f2",
		Code:       "#50a14f",
		Link:       "#0184bc",
		BlockQuote: "#696c77",
	},
	"muted": {
		Text:       "#b0b0b0",
		Heading:    "#7a9ec2",
		Code:       "#7a9e7a",
		Link:       "#7a9eb0",
		BlockQuote: "#888888",
	},
}

// colorfulPreset is a sentinel — it uses glamour's built-in "dark" style
// directly (the original rendering before theme customization).
const colorfulPreset = "colorful"

// NewMarkdownRendererWithPreset creates a renderer using a named preset.
// The special name "notty" uses glamour's notty style (no custom colors).
// Returns nil if the preset name is unknown.
func NewMarkdownRendererWithPreset(width int, name string) *MarkdownRenderer {
	if width < 20 {
		width = 20
	}

	// "colorful" uses glamour's built-in dark style — the original rendering
	// before any theme customization was added.
	if name == colorfulPreset {
		return NewMarkdownRenderer(width)
	}

	// GLAMOUR_STYLE env var takes precedence over presets.
	if envStyle := os.Getenv("GLAMOUR_STYLE"); envStyle != "" && envStyle != "auto" {
		return NewMarkdownRenderer(width)
	}

	if name == "notty" {
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("notty"),
			glamour.WithWordWrap(width),
		)
		if r == nil || err != nil {
			return &MarkdownRenderer{width: width}
		}
		return &MarkdownRenderer{renderer: r, width: width}
	}

	colors, ok := MarkdownPresets[name]
	if !ok {
		return nil
	}
	return NewMarkdownRendererWithTheme(width, colors)
}

// NewMarkdownRendererWithTheme creates a renderer using theme-derived glamour
// colors. If GLAMOUR_STYLE env var is set (and not "auto"), it takes
// precedence and theme colors are ignored.
func NewMarkdownRendererWithTheme(width int, colors MarkdownThemeColors) *MarkdownRenderer {
	if width < 20 {
		width = 20
	}

	// GLAMOUR_STYLE env var takes precedence over theme colors.
	if envStyle := os.Getenv("GLAMOUR_STYLE"); envStyle != "" && envStyle != "auto" {
		return NewMarkdownRenderer(width)
	}

	// If no theme colors provided, fall back to default.
	if !colors.HasColors() {
		return NewMarkdownRenderer(width)
	}

	// Start from the dark base style and override with theme colors.
	sc := styles.DarkStyleConfig
	if colors.Text != "" {
		sc.Document.Color = strPtr(colors.Text)
	}
	if colors.Heading != "" {
		sc.Heading.Color = strPtr(colors.Heading)
		// Override per-level headings — glamour's dark style sets H1 with its
		// own Color ("228") and BackgroundColor ("63") that override the generic
		// Heading color. Clear H1's background and propagate theme color to all levels.
		sc.H1.Color = strPtr(colors.Heading)
		sc.H1.BackgroundColor = strPtr("")
		sc.H2.Color = strPtr(colors.Heading)
		sc.H3.Color = strPtr(colors.Heading)
		sc.H4.Color = strPtr(colors.Heading)
		sc.H5.Color = strPtr(colors.Heading)
		sc.H6.Color = strPtr(colors.Heading)
	}
	if colors.Code != "" {
		sc.Code.Color = strPtr(colors.Code)
	}
	if colors.Link != "" {
		sc.Link.Color = strPtr(colors.Link)
		// LinkText is the visible text of a link — without this, glamour's
		// default LinkText color overrides the Link color we just set.
		sc.LinkText.Color = strPtr(colors.Link)
	}
	if colors.BlockQuote != "" {
		sc.BlockQuote.Color = strPtr(colors.BlockQuote)
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(sc),
		glamour.WithWordWrap(width),
	)
	if r == nil || err != nil {
		return &MarkdownRenderer{width: width}
	}
	return &MarkdownRenderer{renderer: r, width: width}
}

func strPtr(s string) *string {
	return &s
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
