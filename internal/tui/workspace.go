package tui

// Workspace tracks working directories for multi-directory sessions.
type Workspace struct {
	directories []string
	active      int
}

// NewWorkspace creates a Workspace with the given initial directory.
func NewWorkspace(dir string) Workspace {
	if dir == "" {
		return Workspace{}
	}
	return Workspace{
		directories: []string{dir},
		active:      0,
	}
}

// Add appends a directory if not already present.
func (w *Workspace) Add(dir string) {
	if dir == "" {
		return
	}
	for _, d := range w.directories {
		if d == dir {
			return
		}
	}
	w.directories = append(w.directories, dir)
}

// Switch sets the active directory by index. Returns false if out of range.
func (w *Workspace) Switch(index int) bool {
	if index < 0 || index >= len(w.directories) {
		return false
	}
	w.active = index
	return true
}

// Active returns the current working directory, or empty string if none.
func (w Workspace) Active() string {
	if len(w.directories) == 0 {
		return ""
	}
	return w.directories[w.active]
}

// List returns all tracked directories.
func (w Workspace) List() []string {
	out := make([]string, len(w.directories))
	copy(out, w.directories)
	return out
}

// ActiveIndex returns the index of the active directory.
func (w Workspace) ActiveIndex() int {
	return w.active
}

// WorkspaceChangedMsg is sent when the active workspace directory changes.
type WorkspaceChangedMsg struct {
	Path string
}
