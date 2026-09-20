package tui

import (
	"strings"
	"testing"
)

func TestRenderTree_EmptySessions(t *testing.T) {
	result := renderTree(nil, "")
	if result != "" {
		t.Errorf("expected empty string for nil sessions, got %q", result)
	}

	result = renderTree([]SessionInfo{}, "")
	if result != "" {
		t.Errorf("expected empty string for empty sessions, got %q", result)
	}
}

func TestRenderTree_FlatSessionsNoConnectors(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "s1", Title: "First"},
		{ID: "s2", Title: "Second"},
		{ID: "s3", Title: "Third"},
	}

	result := renderTree(sessions, "")

	// Flat sessions should not have tree connectors.
	if strings.Contains(result, "├──") {
		t.Errorf("flat sessions should not contain ├── connector, got:\n%s", result)
	}
	if strings.Contains(result, "└──") {
		t.Errorf("flat sessions should not contain └── connector, got:\n%s", result)
	}
	if strings.Contains(result, "│") {
		t.Errorf("flat sessions should not contain │ connector, got:\n%s", result)
	}

	// All titles should appear.
	for _, title := range []string{"First", "Second", "Third"} {
		if !strings.Contains(result, title) {
			t.Errorf("expected title %q in output, got:\n%s", title, result)
		}
	}
}

func TestRenderTree_ParentChildUsesConnectors(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "root", Title: "Root Session"},
		{ID: "child1", Title: "Child One", ParentID: "root"},
		{ID: "child2", Title: "Child Two", ParentID: "root"},
	}

	result := renderTree(sessions, "")

	if !strings.Contains(result, "├──") {
		t.Errorf("parent-child tree should contain ├── connector, got:\n%s", result)
	}
	if !strings.Contains(result, "└──") {
		t.Errorf("parent-child tree should contain └── connector, got:\n%s", result)
	}

	// Verify ordering: ├── for non-last child, └── for last child.
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	foundBranch := false
	foundEnd := false
	for _, line := range lines {
		if strings.Contains(line, "├──") && strings.Contains(line, "Child One") {
			foundBranch = true
		}
		if strings.Contains(line, "└──") && strings.Contains(line, "Child Two") {
			foundEnd = true
		}
	}
	if !foundBranch {
		t.Errorf("expected Child One with ├── connector, got:\n%s", result)
	}
	if !foundEnd {
		t.Errorf("expected Child Two with └── connector, got:\n%s", result)
	}
}

func TestRenderTree_ActiveSessionMarked(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "s1", Title: "First"},
		{ID: "s2", Title: "Second"},
	}

	result := renderTree(sessions, "s2")

	if !strings.Contains(result, "▸") {
		t.Errorf("active session should be marked with ▸, got:\n%s", result)
	}

	// The active marker should be on the line with "Second".
	lines := strings.Split(result, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "Second") && strings.Contains(line, "▸") {
			found = true
		}
	}
	if !found {
		t.Errorf("▸ marker should appear on the active session line, got:\n%s", result)
	}
}

func TestRenderTree_InactiveSessionNotMarked(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "s1", Title: "First"},
	}

	result := renderTree(sessions, "other")

	// The marker should not appear for non-active sessions.
	if strings.Contains(result, "▸") {
		t.Errorf("non-active session should not have ▸ marker, got:\n%s", result)
	}
}

func TestRenderTree_DeepNesting(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "root", Title: "Root"},
		{ID: "child", Title: "Child", ParentID: "root"},
		{ID: "grandchild", Title: "Grandchild", ParentID: "child"},
	}

	result := renderTree(sessions, "")

	if !strings.Contains(result, "Root") {
		t.Errorf("expected Root in output, got:\n%s", result)
	}
	if !strings.Contains(result, "Child") {
		t.Errorf("expected Child in output, got:\n%s", result)
	}
	if !strings.Contains(result, "Grandchild") {
		t.Errorf("expected Grandchild in output, got:\n%s", result)
	}

	// Grandchild should have deeper indentation than Child.
	lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
	childIndent := -1
	grandchildIndent := -1
	for _, line := range lines {
		if strings.Contains(line, "Child") && !strings.Contains(line, "Grand") {
			childIndent = strings.Index(line, "Child")
		}
		if strings.Contains(line, "Grandchild") {
			grandchildIndent = strings.Index(line, "Grandchild")
		}
	}
	if grandchildIndent <= childIndent {
		t.Errorf("grandchild should be more indented than child (grandchild=%d, child=%d), got:\n%s",
			grandchildIndent, childIndent, result)
	}
}

func TestSidebar_ToggleFlips(t *testing.T) {
	s := NewSidebar()

	if s.IsOpen() {
		t.Error("sidebar should start closed")
	}

	s.Toggle()
	if !s.IsOpen() {
		t.Error("sidebar should be open after first toggle")
	}

	s.Toggle()
	if s.IsOpen() {
		t.Error("sidebar should be closed after second toggle")
	}
}

func TestSidebar_ViewEmptyWhenClosed(t *testing.T) {
	s := NewSidebar()
	s.SetSessions([]SessionInfo{{ID: "s1", Title: "Test"}})

	view := s.View()
	if view != "" {
		t.Errorf("closed sidebar should render empty, got %q", view)
	}
}

func TestSidebar_ViewShowsMetadata(t *testing.T) {
	s := NewSidebar()
	s.Toggle()
	s.SetSize(42, 20)
	s.SetAgent("build")
	s.SetModel("qwen3:8b")
	s.SetSessions([]SessionInfo{{ID: "s1", Title: "Test Session"}})

	view := s.View()

	if !strings.Contains(view, "build") {
		t.Errorf("sidebar should show agent name, got:\n%s", view)
	}
	if !strings.Contains(view, "qwen3:8b") {
		t.Errorf("sidebar should show model name, got:\n%s", view)
	}
	if !strings.Contains(view, "Sessions") {
		t.Errorf("sidebar should show Sessions header, got:\n%s", view)
	}
}

func TestSidebar_WidthBelowThresholdHidden(t *testing.T) {
	// The layout.go sidebarThreshold=120 controls whether sidebar is shown.
	// Verify that calculateLayout returns hasSidebar=false when width<=120.
	l := calculateLayout(100, 40, true)
	if l.hasSidebar {
		t.Error("sidebar should be hidden when width <= 120")
	}

	l = calculateLayout(121, 40, true)
	if !l.hasSidebar {
		t.Error("sidebar should be visible when width > 120")
	}
}

func TestRenderTree_OrphanedChildTreatedAsRoot(t *testing.T) {
	// A child whose parent is not in the list should render as a root.
	sessions := []SessionInfo{
		{ID: "s1", Title: "Orphan", ParentID: "missing-parent"},
	}

	result := renderTree(sessions, "")

	if !strings.Contains(result, "Orphan") {
		t.Errorf("orphaned session should still render, got:\n%s", result)
	}
	if strings.Contains(result, "├──") || strings.Contains(result, "└──") {
		t.Errorf("orphaned session should render as root without connectors, got:\n%s", result)
	}
}

func TestRenderTree_FallsBackToIDWhenTitleEmpty(t *testing.T) {
	sessions := []SessionInfo{
		{ID: "abc123", Title: ""},
	}

	result := renderTree(sessions, "")

	if !strings.Contains(result, "abc123") {
		t.Errorf("session with empty title should show ID, got:\n%s", result)
	}
}

func TestSidebar_ViewShowsContextSection(t *testing.T) {
	s := NewSidebar()
	s.Toggle()
	s.SetSize(42, 20)
	s.SetContext(ContextStats{
		Tokens:       50000,
		ContextLimit: 200000,
		Percent:      25,
		Cost:         0.0123,
	})

	view := s.View()

	if !strings.Contains(view, "Context") {
		t.Errorf("sidebar should show Context header, got:\n%s", view)
	}
	if !strings.Contains(view, "50k tokens") {
		t.Errorf("sidebar should show token count, got:\n%s", view)
	}
	if !strings.Contains(view, "25% used") {
		t.Errorf("sidebar should show percentage used, got:\n%s", view)
	}
	if !strings.Contains(view, "$0.0123 spent") {
		t.Errorf("sidebar should show cost, got:\n%s", view)
	}
}

func TestSidebar_ViewOmitsCostWhenZero(t *testing.T) {
	s := NewSidebar()
	s.Toggle()
	s.SetSize(42, 20)
	s.SetContext(ContextStats{
		Tokens:  1000,
		Percent: 5,
		Cost:    0,
	})

	view := s.View()

	if strings.Contains(view, "spent") {
		t.Errorf("sidebar should not show cost when zero, got:\n%s", view)
	}
}

func TestSidebar_ViewShowsMCPSection(t *testing.T) {
	s := NewSidebar()
	s.Toggle()
	s.SetSize(42, 20)
	s.SetMCPServers([]MCPServer{
		{Name: "filesystem", Status: "connected", ToolCount: 5},
		{Name: "github", Status: "error", Error: "auth failed"},
	})

	view := s.View()

	if !strings.Contains(view, "MCP") {
		t.Errorf("sidebar should show MCP header, got:\n%s", view)
	}
	if !strings.Contains(view, "filesystem") {
		t.Errorf("sidebar should show server name 'filesystem', got:\n%s", view)
	}
	if !strings.Contains(view, "5 tools") {
		t.Errorf("sidebar should show tool count for connected server, got:\n%s", view)
	}
	if !strings.Contains(view, "github") {
		t.Errorf("sidebar should show server name 'github', got:\n%s", view)
	}
	if !strings.Contains(view, "auth failed") {
		t.Errorf("sidebar should show error message, got:\n%s", view)
	}
}

func TestSidebar_ViewOmitsMCPWhenEmpty(t *testing.T) {
	s := NewSidebar()
	s.Toggle()
	s.SetSize(42, 20)

	view := s.View()

	if strings.Contains(view, "MCP") {
		t.Errorf("sidebar should not show MCP section when no servers, got:\n%s", view)
	}
}

func TestSidebar_UpdateMCPServer(t *testing.T) {
	s := NewSidebar()

	// Add new server.
	s.UpdateMCPServer(MCPServer{Name: "fs", Status: "connecting"})
	if len(s.mcpServers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(s.mcpServers))
	}
	if s.mcpServers[0].Status != "connecting" {
		t.Errorf("expected status 'connecting', got %q", s.mcpServers[0].Status)
	}

	// Update existing server.
	s.UpdateMCPServer(MCPServer{Name: "fs", Status: "connected", ToolCount: 3})
	if len(s.mcpServers) != 1 {
		t.Fatalf("expected 1 server after update, got %d", len(s.mcpServers))
	}
	if s.mcpServers[0].Status != "connected" {
		t.Errorf("expected status 'connected', got %q", s.mcpServers[0].Status)
	}
	if s.mcpServers[0].ToolCount != 3 {
		t.Errorf("expected toolCount 3, got %d", s.mcpServers[0].ToolCount)
	}

	// Add different server.
	s.UpdateMCPServer(MCPServer{Name: "github", Status: "error", Error: "timeout"})
	if len(s.mcpServers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(s.mcpServers))
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{500, "500"},
		{999, "999"},
		{1000, "1k"},
		{50000, "50k"},
		{999999, "999k"},
		{1000000, "1.0M"},
		{1500000, "1.5M"},
	}
	for _, tt := range tests {
		got := formatTokens(tt.input)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
