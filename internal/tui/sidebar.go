package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Sidebar msg types.

// SidebarToggleMsg requests toggling sidebar visibility.
type SidebarToggleMsg struct{}

// SidebarSessionSelectedMsg is emitted when a session is selected in the sidebar tree.
type SidebarSessionSelectedMsg struct {
	SessionID string
}

const sidebarWidth = 42

// Sidebar is a toggleable right panel showing the session tree and metadata.
type Sidebar struct {
	sessions []SessionInfo
	active   string
	agent    string
	model    string
	open     bool
	width    int
	height   int
}

// NewSidebar creates a Sidebar.
func NewSidebar() Sidebar {
	return Sidebar{
		width: sidebarWidth,
	}
}

// SetSessions updates the session list.
func (s *Sidebar) SetSessions(sessions []SessionInfo) {
	s.sessions = sessions
}

// SetAgent updates the displayed agent name.
func (s *Sidebar) SetAgent(agent string) {
	s.agent = agent
}

// SetModel updates the displayed model name.
func (s *Sidebar) SetModel(model string) {
	s.model = model
}

// Toggle flips sidebar visibility.
func (s *Sidebar) Toggle() {
	s.open = !s.open
}

// SetSize updates sidebar dimensions.
func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height
}

// IsOpen reports whether the sidebar is visible.
func (s Sidebar) IsOpen() bool {
	return s.open
}

// Init implements tea.Model.
func (s Sidebar) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (s Sidebar) Update(msg tea.Msg) (Sidebar, tea.Cmd) {
	if !s.open {
		return s, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return s, nil
	}

	switch keyMsg.String() {
	case "esc", "q":
		s.open = false
	}

	return s, nil
}

// View implements tea.Model.
func (s Sidebar) View() string {
	if !s.open {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("Sessions\n\n")
	sb.WriteString(renderTree(s.sessions, s.active))

	// Metadata below tree.
	if s.agent != "" || s.model != "" {
		sb.WriteString("\n")
		if s.agent != "" {
			sb.WriteString(styleMetadata.Render("Agent: ") + s.agent + "\n")
		}
		if s.model != "" {
			sb.WriteString(styleMetadata.Render("Model: ") + s.model + "\n")
		}
	}

	content := sb.String()
	return DefaultTheme().SidebarBox.
		Width(s.width - 2). // account for border
		Height(s.height).
		Render(content)
}

// treeNode is a session with its children for tree rendering.
type treeNode struct {
	session  SessionInfo
	children []*treeNode
}

// renderTree renders sessions as an ASCII tree with connectors.
// Root sessions (no ParentID) are listed at the top level.
// Child sessions are nested under their parent with ├──/└──/│ connectors.
// The active session is marked with ▸.
func renderTree(sessions []SessionInfo, active string) string {
	if len(sessions) == 0 {
		return ""
	}

	// Build lookup and identify roots.
	byID := make(map[string]*treeNode, len(sessions))
	var roots []*treeNode

	for i := range sessions {
		byID[sessions[i].ID] = &treeNode{session: sessions[i]}
	}

	for i := range sessions {
		node := byID[sessions[i].ID]
		if sessions[i].ParentID != "" {
			if parent, ok := byID[sessions[i].ParentID]; ok {
				parent.children = append(parent.children, node)
				continue
			}
		}
		roots = append(roots, node)
	}

	var sb strings.Builder
	for i, root := range roots {
		renderNode(&sb, root, "", i == len(roots)-1, active, true)
	}

	return sb.String()
}

// renderNode recursively renders a tree node with ASCII connectors.
func renderNode(sb *strings.Builder, node *treeNode, prefix string, isLast bool, active string, isRoot bool) {
	title := node.session.Title
	if title == "" {
		title = node.session.ID
	}

	marker := "  "
	if node.session.ID == active {
		marker = lipgloss.NewStyle().Bold(true).Render("▸ ")
	}

	if isRoot {
		sb.WriteString(marker + title + "\n")
	} else {
		connector := "├── "
		if isLast {
			connector = "└── "
		}
		sb.WriteString(prefix + connector + marker + title + "\n")
	}

	childPrefix := prefix
	if !isRoot {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, child := range node.children {
		renderNode(sb, child, childPrefix, i == len(node.children)-1, active, false)
	}
}
