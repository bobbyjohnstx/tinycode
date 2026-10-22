package tui

import (
	"strings"
	"testing"
)

func TestKeybindingPaletteItems_HasSections(t *testing.T) {
	keys := DefaultKeyMap()
	items := keybindingPaletteItems(keys)

	sections := 0
	for _, item := range items {
		if strings.HasPrefix(item.Label, "──") {
			sections++
		}
	}
	if sections != 3 {
		t.Errorf("expected 3 section separators (Prompt, Leader, Global), got %d", sections)
	}
}

func TestKeybindingPaletteItems_AllInformational(t *testing.T) {
	keys := DefaultKeyMap()
	items := keybindingPaletteItems(keys)

	for _, item := range items {
		if item.Value != "" {
			t.Errorf("keybinding item %q should have empty Value, got %q", item.Label, item.Value)
		}
	}
}

func TestKeybindingPaletteItems_ContainsExpectedBindings(t *testing.T) {
	keys := DefaultKeyMap()
	items := keybindingPaletteItems(keys)

	expected := map[string]bool{
		"enter":       false,
		"shift+enter": false,
		"tab":         false,
		"ctrl+x b":    false,
		"ctrl+x a":    false,
		"ctrl+x m":    false,
		"ctrl+x o":    false,
		"ctrl+x n":    false,
		"ctrl+x x":    false,
		"ctrl+x i":    false,
		"ctrl+p":      false,
		"ctrl+k":      false,
		"ctrl+f":      false,
		"ctrl+c":      false,
		"ctrl+d":      false,
		"esc":         false,
		"@":           false,
		"/":           false,
	}

	for _, item := range items {
		if _, ok := expected[item.Label]; ok {
			expected[item.Label] = true
		}
	}

	for label, found := range expected {
		if !found {
			t.Errorf("expected keybinding %q not found in palette items", label)
		}
	}
}

func TestKeybindingPaletteItems_IncludesMCPListAndNoF2(t *testing.T) {
	keys := DefaultKeyMap()
	items := keybindingPaletteItems(keys)

	foundMCP := false
	for _, item := range items {
		if item.Label == "ctrl+x i" {
			foundMCP = true
		}
		if item.Label == "f2" || item.Label == "shift+f2" {
			t.Errorf("dead CycleModel binding %q must not appear in help", item.Label)
		}
	}
	if !foundMCP {
		t.Error("expected MCPList (ctrl+x i) in keybinding help")
	}
}

func TestKeyMapHelp_NoUnboundCycleModel(t *testing.T) {
	keys := DefaultKeyMap()
	// Drift guard: palette help must only surface live KeyMap bindings.
	for _, item := range keybindingPaletteItems(keys) {
		if item.Label == "f2" || strings.Contains(item.Description, "next model") {
			t.Fatalf("unexpected cycle-model help entry: %+v", item)
		}
	}
	if strings.Contains(helpCommandsText, "F2") || strings.Contains(helpCommandsText, "f2") {
		t.Error("/help must not document unbound f2 CycleModel")
	}
	if !strings.Contains(helpCommandsText, "PgUp") {
		t.Error("/help should document PgUp/PgDn")
	}
	if !strings.Contains(helpCommandsText, "Alt+Enter") {
		t.Error("/help should document Alt+Enter")
	}
}

func TestKeybindingPaletteItems_SectionOrder(t *testing.T) {
	keys := DefaultKeyMap()
	items := keybindingPaletteItems(keys)

	var sectionLabels []string
	for _, item := range items {
		if strings.HasPrefix(item.Label, "──") {
			sectionLabels = append(sectionLabels, item.Label)
		}
	}

	if len(sectionLabels) < 3 {
		t.Fatalf("expected at least 3 sections, got %d", len(sectionLabels))
	}
	if !strings.Contains(sectionLabels[0], "Prompt") {
		t.Errorf("first section should be Prompt, got %q", sectionLabels[0])
	}
	if !strings.Contains(sectionLabels[1], "Leader") {
		t.Errorf("second section should be Leader, got %q", sectionLabels[1])
	}
	if !strings.Contains(sectionLabels[2], "Global") {
		t.Errorf("third section should be Global, got %q", sectionLabels[2])
	}
}
