package tui

import (
	"strings"
	"testing"
)

func TestWarningList_AddAndAll(t *testing.T) {
	wl := NewWarningList()
	wl.Add("config", "missing key")
	wl.Add("provider", "timeout on discovery")

	all := wl.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(all))
	}
	if all[0].Source != "config" || all[0].Text != "missing key" {
		t.Errorf("first warning = %+v", all[0])
	}
	if all[1].Source != "provider" || all[1].Text != "timeout on discovery" {
		t.Errorf("second warning = %+v", all[1])
	}
}

func TestWarningList_Clear(t *testing.T) {
	wl := NewWarningList()
	wl.Add("test", "one")
	wl.Add("test", "two")
	wl.Clear()

	if wl.Len() != 0 {
		t.Fatalf("expected 0 warnings after clear, got %d", wl.Len())
	}
}

func TestWarningList_CapAt50(t *testing.T) {
	wl := NewWarningList()
	for i := 0; i < 51; i++ {
		wl.Add("test", strings.Repeat("x", i))
	}
	if wl.Len() != 50 {
		t.Fatalf("expected 50 warnings, got %d", wl.Len())
	}
	// First entry should be the second one added (index 1), since index 0 was dropped.
	all := wl.All()
	if all[0].Text != "x" {
		t.Errorf("oldest warning should be dropped; first = %q", all[0].Text)
	}
}

func TestWarningList_Render(t *testing.T) {
	wl := NewWarningList()
	if r := wl.Render(); r != "No warnings." {
		t.Errorf("empty render = %q", r)
	}
	wl.Add("config", "bad key")
	rendered := wl.Render()
	if !strings.Contains(rendered, "config: bad key") {
		t.Errorf("render missing warning, got %q", rendered)
	}
}

func TestHandleClientCommand_WarningsOpensDialog(t *testing.T) {
	app := NewApp("")
	app.warnings.Add("test", "first warning")
	app.warnings.Add("test", "second warning")

	_, handled := app.handleClientCommand("warnings")
	if !handled {
		t.Fatal("expected handled")
	}
	if !app.debugDlg.IsVisible() {
		t.Error("expected debug dialog visible for warnings")
	}
	if app.focus != FocusDialog {
		t.Errorf("expected FocusDialog, got %v", app.focus)
	}
}

func TestHandleClientCommand_WarningsClearRemovesAll(t *testing.T) {
	app := NewApp("")
	app.warnings.Add("test", "one")
	app.warnings.Add("test", "two")

	_, handled := app.handleClientCommand("warnings-clear")
	if !handled {
		t.Fatal("expected handled")
	}
	if app.warnings.Len() != 0 {
		t.Fatalf("expected 0 warnings after clear, got %d", app.warnings.Len())
	}
}
