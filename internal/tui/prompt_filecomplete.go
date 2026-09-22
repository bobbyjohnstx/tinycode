package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FileItem represents a single file or directory for file completion.
type FileItem struct {
	Path  string
	IsDir bool
}

// FileCompletionMsg delivers file listing results to the prompt.
type FileCompletionMsg struct {
	Items []FileItem
	Query string
}

// FileCompleter provides @-triggered file path completion.
type FileCompleter struct {
	visible    bool
	items      []FileItem
	cursor     int
	query      string
	lastQuery  *string // nil means no previous query
	tokenStart int
	tokenEnd   int
	cwd        string
	width      int
}

// NewFileCompleter creates a FileCompleter with the given cwd.
func NewFileCompleter(cwd string) FileCompleter {
	return FileCompleter{cwd: cwd}
}

// SetCwd updates the working directory for file listing.
func (fc *FileCompleter) SetCwd(cwd string) {
	fc.cwd = cwd
}

// SetWidth sets the popover width.
func (fc *FileCompleter) SetWidth(w int) {
	fc.width = w
}

// IsVisible returns whether the file completer popover is showing.
func (fc *FileCompleter) IsVisible() bool {
	return fc.visible
}

// Selected returns the currently highlighted file item and whether one exists.
func (fc *FileCompleter) Selected() (FileItem, bool) {
	if !fc.visible || len(fc.items) == 0 {
		return FileItem{}, false
	}
	if fc.cursor < 0 || fc.cursor >= len(fc.items) {
		return FileItem{}, false
	}
	return fc.items[fc.cursor], true
}

// TokenBounds returns the start and end character positions of the @token.
func (fc *FileCompleter) TokenBounds() (int, int) {
	return fc.tokenStart, fc.tokenEnd
}

// UpdateAtCursor checks for an @token at the cursor position and triggers
// file listing if the query changed. Returns a tea.Cmd if a listing is needed.
func (fc *FileCompleter) UpdateAtCursor(text string, cursorPos int) tea.Cmd {
	query, start, end, found := extractAtToken(text, cursorPos)
	if !found {
		fc.visible = false
		fc.cursor = 0
		fc.query = ""
		fc.lastQuery = nil
		return nil
	}

	fc.tokenStart = start
	fc.tokenEnd = end
	fc.query = query

	if fc.lastQuery == nil || query != *fc.lastQuery {
		fc.lastQuery = &query
		cwd := fc.cwd
		return listFiles(cwd, query)
	}
	return nil
}

// SetResults updates the file list if the query matches the current query.
func (fc *FileCompleter) SetResults(items []FileItem, query string) {
	if query != fc.query {
		return // stale result
	}
	fc.items = items
	fc.visible = len(items) > 0
	if fc.cursor >= len(fc.items) {
		fc.cursor = max(0, len(fc.items)-1)
	}
}

// Update handles keyboard input when the file completer is visible.
// Returns the updated FileCompleter, an optional tea.Cmd, and whether
// the key was consumed.
func (fc FileCompleter) Update(msg tea.KeyMsg) (FileCompleter, tea.Cmd, bool) {
	if !fc.visible {
		return fc, nil, false
	}

	switch msg.String() {
	case "up":
		if fc.cursor > 0 {
			fc.cursor--
		} else {
			fc.cursor = max(0, len(fc.items)-1)
		}
		return fc, nil, true

	case "down":
		if fc.cursor < len(fc.items)-1 {
			fc.cursor++
		} else {
			fc.cursor = 0
		}
		return fc, nil, true

	case "tab", "enter":
		return fc, nil, true

	case "esc":
		fc.visible = false
		fc.cursor = 0
		fc.query = ""
		fc.lastQuery = nil
		return fc, nil, true
	}

	return fc, nil, false
}

// View renders the file completer popover.
func (fc FileCompleter) View() string {
	if !fc.visible || len(fc.items) == 0 {
		return ""
	}

	w := fc.width
	if w <= 0 {
		w = 40
	}

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})
	highlightStyle := lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"}).
		Bold(true)
	dirStyle := lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#0055AA", Dark: "#58A6FF"})
	highlightDirStyle := lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"}).
		Foreground(lipgloss.AdaptiveColor{Light: "#0055AA", Dark: "#58A6FF"}).
		Bold(true)

	nameCol := 0
	for _, item := range fc.items {
		display := item.Path
		if item.IsDir {
			display += "/"
		}
		if n := len(display); n > nameCol {
			nameCol = n
		}
	}
	nameCol += 2

	descCol := w - nameCol - 6
	if descCol < 6 {
		descCol = 6
	}

	var lines []string
	for i, item := range fc.items {
		display := item.Path
		kind := "file"
		if item.IsDir {
			display += "/"
			kind = "dir"
		}

		var nameStr, descStr string
		if i == fc.cursor {
			if item.IsDir {
				nameStr = highlightDirStyle.Width(nameCol).Render(display)
			} else {
				nameStr = highlightStyle.Width(nameCol).Render(display)
			}
			descStr = lipgloss.NewStyle().
				Background(lipgloss.AdaptiveColor{Light: "#E8E8E8", Dark: "#2A2A2A"}).
				Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#999999"}).
				Width(descCol).Render(kind)
		} else {
			if item.IsDir {
				nameStr = dirStyle.Width(nameCol).Render(display)
			} else {
				nameStr = lipgloss.NewStyle().Width(nameCol).Render(display)
			}
			descStr = dimStyle.Width(descCol).Render(kind)
		}

		row := lipgloss.JoinHorizontal(lipgloss.Top, nameStr, descStr)
		lines = append(lines, row)
	}

	return styleDialogBorder.Width(w).Render(
		strings.Join(lines, "\n"),
	)
}

// extractAtToken finds an @-token at the given cursor position.
// The @ must be at position 0 or preceded by whitespace.
// Returns the path fragment (without @), start position (of @), end position, and whether found.
func extractAtToken(text string, cursorPos int) (query string, start int, end int, found bool) {
	runes := []rune(text)
	if cursorPos > len(runes) {
		cursorPos = len(runes)
	}

	// Scan backward from cursorPos to find @
	atPos := -1
	for i := cursorPos - 1; i >= 0; i-- {
		if runes[i] == ' ' || runes[i] == '\t' || runes[i] == '\n' {
			break // hit whitespace before finding @
		}
		if runes[i] == '@' {
			// @ must be at start or preceded by whitespace
			if i == 0 || runes[i-1] == ' ' || runes[i-1] == '\t' || runes[i-1] == '\n' {
				atPos = i
			}
			break
		}
	}

	if atPos < 0 {
		return "", 0, 0, false
	}

	// Token extends forward from @ to next whitespace or end
	tokenEnd := atPos + 1
	for tokenEnd < len(runes) {
		if runes[tokenEnd] == ' ' || runes[tokenEnd] == '\t' || runes[tokenEnd] == '\n' {
			break
		}
		tokenEnd++
	}

	// Only trigger if cursor is within the token
	if cursorPos < atPos || cursorPos > tokenEnd {
		return "", 0, 0, false
	}

	q := string(runes[atPos+1 : tokenEnd])
	return q, atPos, tokenEnd, true
}

// listFiles returns a tea.Cmd that lists files matching the query relative to cwd.
func listFiles(cwd, query string) tea.Cmd {
	return func() tea.Msg {
		dir := cwd
		prefix := ""

		if query == "" {
			// list cwd root
		} else if strings.HasSuffix(query, "/") {
			dir = filepath.Join(cwd, query)
		} else {
			dirPart := filepath.Dir(query)
			prefix = filepath.Base(query)
			if dirPart != "." {
				dir = filepath.Join(cwd, dirPart)
			}
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			return FileCompletionMsg{Items: nil, Query: query}
		}

		var items []FileItem
		lowerPrefix := strings.ToLower(prefix)
		for _, e := range entries {
			name := e.Name()
			// Skip hidden files
			if strings.HasPrefix(name, ".") {
				continue
			}
			if lowerPrefix != "" && !strings.HasPrefix(strings.ToLower(name), lowerPrefix) {
				continue
			}

			// Build the path relative to the query's directory context
			var relPath string
			if query == "" {
				relPath = name
			} else if strings.HasSuffix(query, "/") {
				relPath = query + name
			} else {
				dirPart := filepath.Dir(query)
				if dirPart == "." {
					relPath = name
				} else {
					relPath = dirPart + "/" + name
				}
			}

			items = append(items, FileItem{
				Path:  relPath,
				IsDir: e.IsDir(),
			})
		}

		// Sort: directories first, then alphabetical
		sort.Slice(items, func(i, j int) bool {
			if items[i].IsDir != items[j].IsDir {
				return items[i].IsDir
			}
			return strings.ToLower(items[i].Path) < strings.ToLower(items[j].Path)
		})

		// Cap at 20
		if len(items) > 20 {
			items = items[:20]
		}

		return FileCompletionMsg{Items: items, Query: query}
	}
}

// findAllAtTokens scans text for all @-tokens using the same rules as extractAtToken.
func findAllAtTokens(text string) []atRef {
	runes := []rune(text)
	var refs []atRef

	for i := 0; i < len(runes); i++ {
		if runes[i] != '@' {
			continue
		}
		// @ must be at start or preceded by whitespace
		if i > 0 && runes[i-1] != ' ' && runes[i-1] != '\t' && runes[i-1] != '\n' {
			continue
		}
		// Extract token forward to whitespace
		tokenEnd := i + 1
		for tokenEnd < len(runes) {
			if runes[tokenEnd] == ' ' || runes[tokenEnd] == '\t' || runes[tokenEnd] == '\n' {
				break
			}
			tokenEnd++
		}
		path := string(runes[i+1 : tokenEnd])
		if path == "" {
			continue
		}
		refs = append(refs, atRef{
			path:  path,
			start: i,
			end:   tokenEnd,
		})
		i = tokenEnd - 1 // skip past token
	}
	return refs
}

// atRef represents a single @file reference in prompt text.
type atRef struct {
	path  string
	start int
	end   int
}
