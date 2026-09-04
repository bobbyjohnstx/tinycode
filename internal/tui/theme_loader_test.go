package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTheme_ValidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.json")

	content := `{
		"name": "custom",
		"colors": {
			"accent": "#FF0000",
			"user": "#00FF00",
			"assistant": "#0000FF",
			"error": "#FF6666",
			"success": "#66FF66",
			"subtle": "#888888",
			"background": "#111111",
			"highlight": "#222222"
		}
	}`

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadTheme(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Name != "custom" {
		t.Errorf("expected name 'custom', got %q", cfg.Name)
	}
	if cfg.Colors.Accent != "#FF0000" {
		t.Errorf("expected accent #FF0000, got %q", cfg.Colors.Accent)
	}
	if cfg.Colors.Background != "#111111" {
		t.Errorf("expected background #111111, got %q", cfg.Colors.Background)
	}
}

func TestLoadTheme_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")

	if err := os.WriteFile(path, []byte(`{not json}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadTheme(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadTheme_MissingFile(t *testing.T) {
	_, err := LoadTheme("/nonexistent/theme.json")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadTheme_PartialColors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.json")

	content := `{"name": "partial", "colors": {"accent": "#ABCDEF"}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadTheme(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Colors.Accent != "#ABCDEF" {
		t.Errorf("expected accent #ABCDEF, got %q", cfg.Colors.Accent)
	}
	// Unset colors should be zero values.
	if cfg.Colors.Error != "" {
		t.Errorf("expected empty error color, got %q", cfg.Colors.Error)
	}
}

func TestApplyTheme_NilReturnsDefault(t *testing.T) {
	theme := ApplyTheme(nil)
	def := DefaultTheme()

	// Verify at least one style matches default.
	if theme.Bold.GetBold() != def.Bold.GetBold() {
		t.Error("expected nil config to return default theme")
	}
}

func TestApplyTheme_CustomColors(t *testing.T) {
	cfg := &ThemeConfig{
		Name: "test",
		Colors: ThemeColors{
			Accent: "#AABBCC",
		},
	}

	theme := ApplyTheme(cfg)
	// Theme should not panic and should produce valid styles.
	if theme.UserMessage.GetPaddingLeft() != 2 {
		t.Error("expected user message padding left 2")
	}
}

func TestBuiltinDarkTheme(t *testing.T) {
	cfg := BuiltinDarkTheme()
	if cfg.Name != "dark" {
		t.Errorf("expected name 'dark', got %q", cfg.Name)
	}
	if cfg.Colors.Accent == "" {
		t.Error("expected non-empty accent color")
	}
}

func TestBuiltinLightTheme(t *testing.T) {
	cfg := BuiltinLightTheme()
	if cfg.Name != "light" {
		t.Errorf("expected name 'light', got %q", cfg.Name)
	}
	if cfg.Colors.Accent == "" {
		t.Error("expected non-empty accent color")
	}
}

func TestDetectColorScheme_Default(t *testing.T) {
	t.Setenv("COLORFGBG", "")
	if DetectColorScheme() != "dark" {
		t.Error("expected 'dark' when COLORFGBG is empty")
	}
}

func TestDetectColorScheme_LightBackground(t *testing.T) {
	t.Setenv("COLORFGBG", "0;15")
	if DetectColorScheme() != "light" {
		t.Error("expected 'light' for bg index 15")
	}

	t.Setenv("COLORFGBG", "0;7")
	if DetectColorScheme() != "light" {
		t.Error("expected 'light' for bg index 7")
	}
}

func TestDetectColorScheme_DarkBackground(t *testing.T) {
	t.Setenv("COLORFGBG", "15;0")
	if DetectColorScheme() != "dark" {
		t.Error("expected 'dark' for bg index 0")
	}
}

func TestColorOrDefault(t *testing.T) {
	if colorOrDefault("", "#FFF") != "#FFF" {
		t.Error("expected fallback when empty")
	}
	if colorOrDefault("#000", "#FFF") != "#000" {
		t.Error("expected value when non-empty")
	}
}
