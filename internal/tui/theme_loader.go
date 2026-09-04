package tui

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ThemeConfig is the JSON-serializable theme definition.
type ThemeConfig struct {
	Name   string          `json:"name"`
	Colors ThemeColors     `json:"colors"`
	Styles json.RawMessage `json:"styles,omitempty"` // reserved for future per-component overrides
}

// ThemeColors defines the color palette for a theme.
type ThemeColors struct {
	Accent     string `json:"accent"`
	User       string `json:"user"`
	Assistant  string `json:"assistant"`
	Error      string `json:"error"`
	Success    string `json:"success"`
	Subtle     string `json:"subtle"`
	Background string `json:"background"`
	Highlight  string `json:"highlight"`
}

// LoadTheme reads a theme JSON file and returns the parsed config.
// Returns an error if the file cannot be read or parsed.
func LoadTheme(path string) (*ThemeConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg ThemeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// ApplyTheme converts a ThemeConfig into a Theme with lipgloss styles.
func ApplyTheme(cfg *ThemeConfig) Theme {
	if cfg == nil {
		return DefaultTheme()
	}

	accent := colorOrDefault(cfg.Colors.Accent, "#58A6FF")
	user := colorOrDefault(cfg.Colors.User, "#E1E1E1")
	assistant := colorOrDefault(cfg.Colors.Assistant, "#CCCCCC")
	errorC := colorOrDefault(cfg.Colors.Error, "#FF6666")
	success := colorOrDefault(cfg.Colors.Success, "#66FF66")
	subtle := colorOrDefault(cfg.Colors.Subtle, "#777777")
	bg := colorOrDefault(cfg.Colors.Background, "#1A1A1A")
	highlight := colorOrDefault(cfg.Colors.Highlight, "#2A2A2A")

	return Theme{
		UserMessage: lipgloss.NewStyle().
			Foreground(lipgloss.Color(user)).
			Bold(true).
			PaddingLeft(2),

		AssistantMessage: lipgloss.NewStyle().
			Foreground(lipgloss.Color(assistant)).
			PaddingLeft(2),

		ChatBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(subtle)),

		SidebarBox: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(subtle)).
			Padding(0, 1),

		PromptBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(accent)).
			BorderTop(true),

		StatusBar: lipgloss.NewStyle().
			Background(lipgloss.Color(bg)).
			Foreground(lipgloss.Color(subtle)).
			Padding(0, 1),

		StatusBarModel: lipgloss.NewStyle().
			Foreground(lipgloss.Color(accent)).
			Bold(true),

		StatusBarAgent: lipgloss.NewStyle().
			Foreground(lipgloss.Color(success)),

		StatusBarCwd: lipgloss.NewStyle().
			Foreground(lipgloss.Color(subtle)).
			Italic(true),

		ToastInfo: lipgloss.NewStyle().
			Foreground(lipgloss.Color(accent)).
			Padding(0, 1),

		ToastError: lipgloss.NewStyle().
			Foreground(lipgloss.Color(errorC)).
			Padding(0, 1),

		DialogOverlay: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(accent)).
			Padding(1, 2),

		DialogTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color(accent)).
			Bold(true).
			MarginBottom(1),

		DialogItem: lipgloss.NewStyle().
			PaddingLeft(2),

		DialogActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color(accent)).
			Bold(true).
			PaddingLeft(2),

		PermissionBorder: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#FFAA33")),

		PermissionAllow: lipgloss.NewStyle().
			Foreground(lipgloss.Color(success)).
			Bold(true),

		PermissionDeny: lipgloss.NewStyle().
			Foreground(lipgloss.Color(errorC)).
			Bold(true),

		PaletteInput: lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color(accent)).
			Padding(0, 1),

		PaletteItem: lipgloss.NewStyle().
			PaddingLeft(2),

		PaletteActive: lipgloss.NewStyle().
			Background(lipgloss.Color(highlight)).
			Foreground(lipgloss.Color(accent)).
			Bold(true).
			PaddingLeft(2),

		Spinner: lipgloss.NewStyle().
			Foreground(lipgloss.Color(accent)),

		Dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color(subtle)),

		Bold: lipgloss.NewStyle().
			Bold(true),
	}
}

// BuiltinDarkTheme returns the built-in dark theme config.
func BuiltinDarkTheme() *ThemeConfig {
	return &ThemeConfig{
		Name: "dark",
		Colors: ThemeColors{
			Accent:     "#58A6FF",
			User:       "#E1E1E1",
			Assistant:  "#CCCCCC",
			Error:      "#FF6666",
			Success:    "#66FF66",
			Subtle:     "#777777",
			Background: "#1A1A1A",
			Highlight:  "#2A2A2A",
		},
	}
}

// BuiltinLightTheme returns the built-in light theme config.
func BuiltinLightTheme() *ThemeConfig {
	return &ThemeConfig{
		Name: "light",
		Colors: ThemeColors{
			Accent:     "#0070F3",
			User:       "#1A1A1A",
			Assistant:  "#333333",
			Error:      "#CC0000",
			Success:    "#006600",
			Subtle:     "#999999",
			Background: "#F5F5F5",
			Highlight:  "#E8E8E8",
		},
	}
}

// DetectColorScheme inspects the COLORFGBG environment variable to guess
// whether the terminal uses a dark or light background.
// Returns "dark" or "light".
func DetectColorScheme() string {
	val := os.Getenv("COLORFGBG")
	if val == "" {
		return "dark" // default assumption
	}

	// COLORFGBG format: "fg;bg" where bg is a color index (0-15).
	// Indexes 0-6 are dark backgrounds, 7+ are light.
	parts := strings.SplitN(val, ";", 2)
	if len(parts) < 2 {
		return "dark"
	}

	bg := parts[len(parts)-1]
	// Common light backgrounds: 7 (white), 15 (bright white)
	switch bg {
	case "7", "15":
		return "light"
	default:
		return "dark"
	}
}

// colorOrDefault returns c if non-empty, otherwise fallback.
func colorOrDefault(c, fallback string) string {
	if c == "" {
		return fallback
	}
	return c
}
