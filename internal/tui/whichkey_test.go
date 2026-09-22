package tui

import (
	"strings"
	"testing"
)

func TestWhichKeyPanelShowHide(t *testing.T) {
	var panel WhichKeyPanel

	if panel.IsVisible() {
		t.Fatal("expected panel to be hidden initially")
	}

	entries := []WhichKeyEntry{
		{Key: "b", Description: "sidebar", Category: "Nav"},
	}
	panel.Show(entries)

	if !panel.IsVisible() {
		t.Fatal("expected panel to be visible after Show")
	}

	panel.Hide()

	if panel.IsVisible() {
		t.Fatal("expected panel to be hidden after Hide")
	}
}

func TestWhichKeyPanelViewEmpty(t *testing.T) {
	var panel WhichKeyPanel

	if panel.View() != "" {
		t.Fatal("expected empty view when not visible")
	}

	panel.visible = true
	if panel.View() != "" {
		t.Fatal("expected empty view when visible but no entries")
	}
}

func TestWhichKeyPanelViewRendersEntries(t *testing.T) {
	var panel WhichKeyPanel
	panel.SetSize(80, 24)
	panel.Show([]WhichKeyEntry{
		{Key: "b", Description: "toggle sidebar", Category: "Navigation"},
		{Key: "o", Description: "session list", Category: "Navigation"},
		{Key: "a", Description: "agent list", Category: "Tools"},
	})

	view := panel.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}

	// Verify all keys and descriptions appear in the rendered output.
	for _, want := range []string{"b", "o", "a", "toggle sidebar", "session list", "agent list"} {
		if !strings.Contains(view, want) {
			t.Errorf("expected view to contain %q", want)
		}
	}

	// Verify categories appear.
	for _, cat := range []string{"Navigation", "Tools"} {
		if !strings.Contains(view, cat) {
			t.Errorf("expected view to contain category %q", cat)
		}
	}
}

func TestWhichKeyPanelViewGroupsByCategory(t *testing.T) {
	var panel WhichKeyPanel
	panel.SetSize(80, 24)
	panel.Show([]WhichKeyEntry{
		{Key: "b", Description: "sidebar", Category: "Nav"},
		{Key: "a", Description: "agents", Category: "Tools"},
		{Key: "o", Description: "sessions", Category: "Nav"},
	})

	view := panel.View()

	// Nav should appear before Tools in the output.
	navIdx := strings.Index(view, "Nav")
	toolsIdx := strings.Index(view, "Tools")
	if navIdx < 0 || toolsIdx < 0 {
		t.Fatal("expected both categories in view")
	}
	if navIdx >= toolsIdx {
		t.Error("expected Nav category to appear before Tools category")
	}
}

func TestLeaderKeyEntriesHasAllBindings(t *testing.T) {
	keys := DefaultKeyMap()
	entries := LeaderKeyEntries(keys)

	if len(entries) != 13 {
		t.Fatalf("expected 13 leader entries, got %d", len(entries))
	}

	// Verify expected categories exist.
	cats := make(map[string]bool)
	for _, e := range entries {
		cats[e.Category] = true
	}
	for _, want := range []string{"Navigation", "Edit", "Tools", "Actions"} {
		if !cats[want] {
			t.Errorf("expected category %q in entries", want)
		}
	}

	// Verify all leader keys are present.
	keySet := make(map[string]bool)
	for _, e := range entries {
		keySet[e.Key] = true
	}
	for _, want := range []string{"b", "o", "n", "e", "d", "u", "r", "a", "m", "t", "i", "y", "x"} {
		if !keySet[want] {
			t.Errorf("expected key %q in entries", want)
		}
	}
}
