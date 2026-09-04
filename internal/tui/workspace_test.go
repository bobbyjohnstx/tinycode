package tui

import "testing"

func TestWorkspace_NewEmpty(t *testing.T) {
	w := NewWorkspace("")
	if w.Active() != "" {
		t.Errorf("expected empty active, got %q", w.Active())
	}
	if len(w.List()) != 0 {
		t.Errorf("expected empty list, got %v", w.List())
	}
}

func TestWorkspace_NewWithDir(t *testing.T) {
	w := NewWorkspace("/home/user/project")
	if w.Active() != "/home/user/project" {
		t.Errorf("expected /home/user/project, got %q", w.Active())
	}
	if len(w.List()) != 1 {
		t.Errorf("expected 1 directory, got %d", len(w.List()))
	}
}

func TestWorkspace_AddAndList(t *testing.T) {
	w := NewWorkspace("/a")
	w.Add("/b")
	w.Add("/c")

	dirs := w.List()
	if len(dirs) != 3 {
		t.Fatalf("expected 3 directories, got %d", len(dirs))
	}
	if dirs[0] != "/a" || dirs[1] != "/b" || dirs[2] != "/c" {
		t.Errorf("unexpected order: %v", dirs)
	}
}

func TestWorkspace_AddDuplicate(t *testing.T) {
	w := NewWorkspace("/a")
	w.Add("/a")
	w.Add("/a")

	if len(w.List()) != 1 {
		t.Errorf("expected dedup to 1, got %d", len(w.List()))
	}
}

func TestWorkspace_AddEmpty(t *testing.T) {
	w := NewWorkspace("/a")
	w.Add("")

	if len(w.List()) != 1 {
		t.Errorf("expected 1 after adding empty, got %d", len(w.List()))
	}
}

func TestWorkspace_Switch(t *testing.T) {
	w := NewWorkspace("/a")
	w.Add("/b")
	w.Add("/c")

	if !w.Switch(2) {
		t.Error("expected switch to index 2 to succeed")
	}
	if w.Active() != "/c" {
		t.Errorf("expected /c, got %q", w.Active())
	}
	if w.ActiveIndex() != 2 {
		t.Errorf("expected index 2, got %d", w.ActiveIndex())
	}
}

func TestWorkspace_SwitchOutOfRange(t *testing.T) {
	w := NewWorkspace("/a")

	if w.Switch(-1) {
		t.Error("expected switch to -1 to fail")
	}
	if w.Switch(5) {
		t.Error("expected switch to 5 to fail")
	}
	if w.Active() != "/a" {
		t.Errorf("active should be unchanged, got %q", w.Active())
	}
}

func TestWorkspace_ListReturnsCopy(t *testing.T) {
	w := NewWorkspace("/a")
	w.Add("/b")

	dirs := w.List()
	dirs[0] = "/modified"

	if w.Active() != "/a" {
		t.Error("modifying List() return should not affect workspace")
	}
}
