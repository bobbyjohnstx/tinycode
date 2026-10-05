package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Sidebar msg types.

// SidebarToggleMsg requests toggling sidebar visibility.
type SidebarToggleMsg struct{}

// SidebarSessionSelectedMsg is emitted when a session is selected in the sidebar tree.
type SidebarSessionSelectedMsg = SessionSwitchedMsg

const sidebarWidth = 28

// ContextStats holds computed context usage for sidebar display.
type ContextStats struct {
	Tokens       int
	InputTokens  int
	OutputTokens int
	ContextLimit int
	Percent      int
	Cost         float64
}

// ProviderBalance holds balance info for a provider (e.g., OpenRouter).
type ProviderBalance struct {
	Remaining float64
	Usage     float64
	HasLimit  bool
	Provider  string
}

// MCPServer holds status for a single MCP server.
type MCPServer struct {
	Name      string
	Status    string
	Error     string
	ToolCount int
}

// Sidebar is a toggleable right panel showing the session tree and metadata.
type Sidebar struct {
	sessions   []SessionInfo
	active     string
	agent      string
	model      string
	cwd        string
	version    string
	context    ContextStats
	balance    *ProviderBalance
	mcpServers []MCPServer
	open       bool
	width      int
	height     int
}

// NewSidebar creates a Sidebar.
func NewSidebar() Sidebar {
	return Sidebar{
		width:   sidebarWidth,
		version: "dev",
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

// SetCwd updates the working directory display.
func (s *Sidebar) SetCwd(cwd string) {
	s.cwd = cwd
}

// SetContext updates the context stats display.
func (s *Sidebar) SetContext(stats ContextStats) {
	s.context = stats
}

// SetMCPServers replaces the full MCP server list.
func (s *Sidebar) SetMCPServers(servers []MCPServer) {
	s.mcpServers = servers
}

// SetVersion updates the version display.
func (s *Sidebar) SetVersion(v string) {
	s.version = v
}

// SetBalance updates the provider balance display.
func (s *Sidebar) SetBalance(b *ProviderBalance) {
	s.balance = b
}

// UpdateMCPServer upserts a single MCP server status.
func (s *Sidebar) UpdateMCPServer(srv MCPServer) {
	for i, existing := range s.mcpServers {
		if existing.Name == srv.Name {
			s.mcpServers[i] = srv
			return
		}
	}
	s.mcpServers = append(s.mcpServers, srv)
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

var (
	styleSidebarHeader = lipgloss.NewStyle().Bold(true)
	styleSidebarMuted  = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"})
	styleSidebarSuccess = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#008800", Dark: "#44CC44"})
	styleSidebarError = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#CC0000", Dark: "#FF4444"})
)

// View implements tea.Model.
func (s Sidebar) View() string {
	if !s.open {
		return ""
	}

	var sb strings.Builder

	// Context section
	sb.WriteString(styleSidebarHeader.Render("Context"))
	sb.WriteString("\n")
	if s.context.Tokens > 0 {
		sb.WriteString(styleSidebarMuted.Render(formatTokens(s.context.Tokens) + " tokens"))
		sb.WriteString("\n")
		sb.WriteString(styleSidebarMuted.Render(fmt.Sprintf("%d%% used", s.context.Percent)))
		sb.WriteString("\n")
	} else if s.context.ContextLimit > 0 {
		sb.WriteString(styleSidebarMuted.Render(formatTokens(s.context.ContextLimit) + " limit"))
		sb.WriteString("\n")
	}
	if s.context.InputTokens > 0 || s.context.OutputTokens > 0 {
		sb.WriteString(styleSidebarMuted.Render(
			formatTokens(s.context.InputTokens) + " in / " + formatTokens(s.context.OutputTokens) + " out"))
		sb.WriteString("\n")
	}
	if s.context.Cost > 0 {
		sb.WriteString(styleSidebarMuted.Render(formatCost(s.context.Cost) + " spent"))
		sb.WriteString("\n")
	}
	if s.balance != nil {
		if s.balance.HasLimit {
			sb.WriteString(styleSidebarMuted.Render(formatCost(s.balance.Remaining) + " remaining"))
			sb.WriteString("\n")
		}
		if s.balance.Usage > 0 {
			sb.WriteString(styleSidebarMuted.Render(formatCost(s.balance.Usage) + " used (" + s.balance.Provider + ")"))
			sb.WriteString("\n")
		}
	}

	// MCP section
	if len(s.mcpServers) > 0 {
		sb.WriteString("\n")
		sb.WriteString(styleSidebarHeader.Render("MCP"))
		sb.WriteString("\n")
		for _, srv := range s.mcpServers {
			indicator := styleSidebarMuted.Render("○")
			if srv.Status == "connected" {
				indicator = styleSidebarSuccess.Render("●")
			} else if srv.Status == "error" {
				indicator = styleSidebarError.Render("●")
			}
			sb.WriteString(indicator + " " + srv.Name)
			if srv.Status == "connected" && srv.ToolCount > 0 {
				sb.WriteString(styleSidebarMuted.Render(fmt.Sprintf(" (%d tools)", srv.ToolCount)))
			}
			sb.WriteString("\n")
			if srv.Error != "" {
				sb.WriteString(styleSidebarMuted.Render("  " + srv.Error))
				sb.WriteString("\n")
			}
		}
	}

	// Session tree — skip sessions with empty titles.
	titled := filterTitledSessions(s.sessions)
	if len(titled) > 0 {
		sb.WriteString("\n")
		sb.WriteString(styleSidebarHeader.Render("Sessions"))
		sb.WriteString("\n\n")
		const maxSidebarSessions = 5
		displaySessions := titled
		if len(displaySessions) > maxSidebarSessions {
			displaySessions = displaySessions[:maxSidebarSessions]
		}
		sb.WriteString(renderTree(displaySessions, s.active))
		if len(titled) > maxSidebarSessions {
			sb.WriteString(styleSidebarMuted.Render(fmt.Sprintf("  +%d more", len(titled)-maxSidebarSessions)))
			sb.WriteString("\n")
		}
	}

	// Metadata
	if s.agent != "" || s.model != "" {
		sb.WriteString("\n")
		if s.agent != "" {
			sb.WriteString(styleSidebarMuted.Render("Agent: ") + s.agent + "\n")
		}
		if s.model != "" {
			sb.WriteString(styleSidebarMuted.Render("Model: ") + s.model + "\n")
		}
	}

	// Footer
	footer := s.renderFooter()
	if footer != "" {
		sb.WriteString("\n")
		sb.WriteString(footer)
	}

	content := sb.String()
	return DefaultTheme().SidebarBox.
		Width(s.width - 2).
		Height(s.height).
		Render(content)
}

func (s Sidebar) renderFooter() string {
	var lines []string

	if s.cwd != "" {
		display := s.cwd
		if home, err := os.UserHomeDir(); err == nil {
			display = strings.Replace(display, home, "~", 1)
		}
		dir := filepath.Dir(display)
		base := filepath.Base(display)
		lines = append(lines, styleSidebarMuted.Render(dir+"/")+base)
	}

	lines = append(lines, styleSidebarSuccess.Render("•")+" "+
		styleSidebarMuted.Render("tiny")+
		"code"+
		" "+styleSidebarMuted.Render(s.version))

	return strings.Join(lines, "\n")
}

func filterTitledSessions(sessions []SessionInfo) []SessionInfo {
	var out []SessionInfo
	for _, s := range sessions {
		if s.Title != "" {
			out = append(out, s)
		}
	}
	return out
}

func formatCost(c float64) string {
	switch {
	case c < 0.01:
		return fmt.Sprintf("$%.4f", c)
	case c < 1.0:
		return fmt.Sprintf("$%.3f", c)
	default:
		return fmt.Sprintf("$%.2f", c)
	}
}

func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
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
