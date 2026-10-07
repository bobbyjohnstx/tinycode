package tui

import (
	"embed"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/bobbyjohnstx/tinycode/internal/tui/render"
)

//go:embed themes/*.json
var themesFS embed.FS

type themeJSON struct {
	Schema string                    `json:"$schema"`
	Defs   map[string]string         `json:"defs"`
	Theme  map[string]themeColorPair `json:"theme"`
}

type themeColorPair struct {
	Dark  string `json:"dark"`
	Light string `json:"light"`
}

type ColorTheme struct {
	Name            string
	ID              string
	Primary         lipgloss.AdaptiveColor
	Secondary       lipgloss.AdaptiveColor
	Accent          lipgloss.AdaptiveColor
	Error           lipgloss.AdaptiveColor
	Warning         lipgloss.AdaptiveColor
	Success         lipgloss.AdaptiveColor
	Info            lipgloss.AdaptiveColor
	Text            lipgloss.AdaptiveColor
	TextMuted       lipgloss.AdaptiveColor
	Border          lipgloss.AdaptiveColor
	BorderActive    lipgloss.AdaptiveColor
	BorderSubtle    lipgloss.AdaptiveColor
	BgPanel         lipgloss.AdaptiveColor
	BgElement       lipgloss.AdaptiveColor
	DiffAdded       lipgloss.AdaptiveColor
	DiffRemoved     lipgloss.AdaptiveColor
}

type ThemeRegistry struct {
	themes map[string]*ColorTheme
	order  []string
}

func NewThemeRegistry() *ThemeRegistry {
	return &ThemeRegistry{themes: make(map[string]*ColorTheme)}
}

func (r *ThemeRegistry) LoadEmbedded() {
	entries, err := themesFS.ReadDir("themes")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := themesFS.ReadFile(filepath.Join("themes", entry.Name()))
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		theme := parseThemeJSON(id, data)
		if theme != nil {
			r.themes[id] = theme
		}
	}

	r.order = make([]string, 0, len(r.themes))
	for id := range r.themes {
		r.order = append(r.order, id)
	}
	sort.Strings(r.order)
}

func (r *ThemeRegistry) Get(id string) *ColorTheme {
	return r.themes[id]
}

func (r *ThemeRegistry) List() []ColorTheme {
	result := make([]ColorTheme, 0, len(r.order))
	for _, id := range r.order {
		result = append(result, *r.themes[id])
	}
	return result
}

func parseThemeJSON(id string, data []byte) *ColorTheme {
	var raw themeJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	resolve := func(val string) string {
		if raw.Defs != nil {
			if resolved, ok := raw.Defs[val]; ok {
				return resolved
			}
		}
		return val
	}

	color := func(key string) lipgloss.AdaptiveColor {
		pair, ok := raw.Theme[key]
		if !ok {
			return lipgloss.AdaptiveColor{}
		}
		return lipgloss.AdaptiveColor{
			Dark:  resolve(pair.Dark),
			Light: resolve(pair.Light),
		}
	}

	name := strings.ReplaceAll(id, "-", " ")
	words := strings.Fields(name)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	name = strings.Join(words, " ")

	return &ColorTheme{
		Name:         name,
		ID:           id,
		Primary:      color("primary"),
		Secondary:    color("secondary"),
		Accent:       color("accent"),
		Error:        color("error"),
		Warning:      color("warning"),
		Success:      color("success"),
		Info:         color("info"),
		Text:         color("text"),
		TextMuted:    color("textMuted"),
		Border:       color("border"),
		BorderActive: color("borderActive"),
		BorderSubtle: color("borderSubtle"),
		BgPanel:      color("backgroundPanel"),
		BgElement:    color("backgroundElement"),
		DiffAdded:    color("diffAdded"),
		DiffRemoved:  color("diffRemoved"),
	}
}

func ApplyColorTheme(ct *ColorTheme) {
	styleSpinner = lipgloss.NewStyle().Foreground(ct.Warning)
	colorError = ct.Error
	colorSuccess = ct.Success
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(ct.Primary)
	styleMetadata = lipgloss.NewStyle().Foreground(ct.TextMuted)
	styleDialogBorder = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(ct.Primary).
		Padding(1, 2)
	styleToolName = lipgloss.NewStyle().Bold(true).Foreground(ct.Primary)
	styleUserBorder = lipgloss.NewStyle().
		BorderStyle(lipgloss.ThickBorder()).
		BorderLeft(true).BorderTop(false).BorderRight(false).BorderBottom(false).
		BorderForeground(ct.Secondary).
		PaddingLeft(1)
	styleTimestamp = lipgloss.NewStyle().Foreground(ct.TextMuted)
	styleAgentFooter = lipgloss.NewStyle().Foreground(ct.TextMuted)
	styleReasoningText = lipgloss.NewStyle().Foreground(ct.TextMuted)

	// render package tool name style
	render.SetToolNameStyle(lipgloss.NewStyle().Bold(true).Foreground(ct.Primary))

	// Agent color override
	primary := ct.Primary
	themeAgentColor = &primary

	// Reasoning label uses accent instead of per-agent color
	styleReasoningLabel = lipgloss.NewStyle().Foreground(ct.Accent)

	// Status bar inline styles
	styleStatusDim = lipgloss.NewStyle().Foreground(ct.TextMuted)
	styleStatusAccent = lipgloss.NewStyle().Foreground(ct.Accent)

	// Sidebar styles
	styleSidebarHeader = lipgloss.NewStyle().Bold(true).Foreground(ct.Primary)
	styleSidebarMuted = lipgloss.NewStyle().Foreground(ct.TextMuted)
	styleSidebarSuccess = lipgloss.NewStyle().Foreground(ct.Success)
	styleSidebarError = lipgloss.NewStyle().Foreground(ct.Error)

	// Prompt surface, dim, and primary colors
	colorPromptSurface = ct.BgElement
	colorPromptDim = ct.TextMuted
	colorPromptPrimary = ct.Primary
}
