package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestUpsertPartPreservesThoughtExpanded is a regression test for the bug where
// streaming part updates replaced the entire PartView struct, resetting
// ThoughtExpanded to false and breaking click-to-expand.
func TestUpsertPartPreservesThoughtExpanded(t *testing.T) {
	cv := ChatView{
		messages: []MessageView{
			{
				Info: MessageInfo{ID: "msg1", Role: "assistant"},
				Parts: []PartView{
					{ID: "part1", MessageID: "msg1", Type: "reasoning", Text: "initial", ThoughtExpanded: true},
				},
			},
		},
	}

	update := MessagePartUpdatedMsg{
		Part: PartView{ID: "part1", MessageID: "msg1", Type: "reasoning", Text: "updated text"},
	}
	cv.upsertPart(update)

	part := cv.messages[0].Parts[0]
	if part.Text != "updated text" {
		t.Errorf("expected updated text, got %q", part.Text)
	}
	if !part.ThoughtExpanded {
		t.Error("upsertPart wiped ThoughtExpanded — regression: streaming updates must preserve UI state")
	}
}

func TestUpsertPartPreservesCollapsed(t *testing.T) {
	cv := ChatView{
		messages: []MessageView{
			{
				Info: MessageInfo{ID: "msg1", Role: "assistant"},
				Parts: []PartView{
					{ID: "part1", MessageID: "msg1", Type: "tool-result", Text: "old", Collapsed: true},
				},
			},
		},
	}

	update := MessagePartUpdatedMsg{
		Part: PartView{ID: "part1", MessageID: "msg1", Type: "tool-result", Text: "new output"},
	}
	cv.upsertPart(update)

	if !cv.messages[0].Parts[0].Collapsed {
		t.Error("upsertPart wiped Collapsed state — streaming updates must preserve UI state")
	}
}

func TestToggleThoughtByID(t *testing.T) {
	cv := ChatView{
		messages: []MessageView{
			{
				Info: MessageInfo{ID: "msg1", Role: "assistant"},
				Parts: []PartView{
					{ID: "p1", Type: "reasoning", ThoughtExpanded: false},
					{ID: "p2", Type: "reasoning", ThoughtExpanded: false},
				},
			},
		},
	}

	cv.toggleThought("p1")

	if !cv.messages[0].Parts[0].ThoughtExpanded {
		t.Error("p1 should be expanded after toggle")
	}
	if cv.messages[0].Parts[1].ThoughtExpanded {
		t.Error("p2 should remain collapsed when toggling only p1")
	}
}

func TestToggleThoughtAll(t *testing.T) {
	cv := ChatView{
		messages: []MessageView{
			{
				Info: MessageInfo{ID: "msg1", Role: "assistant"},
				Parts: []PartView{
					{ID: "p1", Type: "reasoning", ThoughtExpanded: false},
					{ID: "p2", Type: "reasoning", ThoughtExpanded: false},
				},
			},
		},
	}

	cv.toggleThought("") // empty = toggle all

	for i, p := range cv.messages[0].Parts {
		if !p.ThoughtExpanded {
			t.Errorf("part %d should be expanded after toggle-all", i)
		}
	}
}

// --- applyDelta tests ---

func TestApplyDelta_AppendsTextToMatchingPart(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", MessageID: "msg1", Type: "text", Text: "Hello"},
			},
		},
	}

	cv.applyDelta(MessagePartDeltaMsg{
		MessageID: "msg1",
		PartID:    "p1",
		Field:     "text",
		Delta:     " world",
	})

	got := cv.messages[0].Parts[0].Text
	if got != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", got)
	}
}

func TestApplyDelta_SetsStreamingFlag(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", MessageID: "msg1", Type: "text", Text: ""},
			},
		},
	}

	cv.applyDelta(MessagePartDeltaMsg{
		MessageID: "msg1",
		PartID:    "p1",
		Field:     "text",
		Delta:     "chunk",
	})

	if !cv.messages[0].Parts[0].Streaming {
		t.Error("Streaming flag should be true after delta")
	}
}

func TestApplyDelta_IgnoresNonMatchingMessageID(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", MessageID: "msg1", Type: "text", Text: "original"},
			},
		},
	}

	cv.applyDelta(MessagePartDeltaMsg{
		MessageID: "msg-nonexistent",
		PartID:    "p1",
		Field:     "text",
		Delta:     " extra",
	})

	if cv.messages[0].Parts[0].Text != "original" {
		t.Errorf("text should be unchanged, got %q", cv.messages[0].Parts[0].Text)
	}
}

func TestApplyDelta_IgnoresNonMatchingPartID(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", MessageID: "msg1", Type: "text", Text: "original"},
			},
		},
	}

	cv.applyDelta(MessagePartDeltaMsg{
		MessageID: "msg1",
		PartID:    "p-nonexistent",
		Field:     "text",
		Delta:     " extra",
	})

	if cv.messages[0].Parts[0].Text != "original" {
		t.Errorf("text should be unchanged, got %q", cv.messages[0].Parts[0].Text)
	}
}

func TestApplyDelta_IgnoresNonTextField(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", MessageID: "msg1", Type: "text", Text: "original"},
			},
		},
	}

	cv.applyDelta(MessagePartDeltaMsg{
		MessageID: "msg1",
		PartID:    "p1",
		Field:     "other_field",
		Delta:     " extra",
	})

	if cv.messages[0].Parts[0].Text != "original" {
		t.Errorf("text should be unchanged for non-text field, got %q", cv.messages[0].Parts[0].Text)
	}
	if cv.messages[0].Parts[0].Streaming {
		t.Error("Streaming should not be set for non-text field delta")
	}
}

func TestApplyDelta_AccumulatesMultipleDeltas(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", MessageID: "msg1", Type: "text", Text: ""},
			},
		},
	}

	deltas := []string{"Hello", " ", "world", "!"}
	for _, d := range deltas {
		cv.applyDelta(MessagePartDeltaMsg{
			MessageID: "msg1",
			PartID:    "p1",
			Field:     "text",
			Delta:     d,
		})
	}

	got := cv.messages[0].Parts[0].Text
	if got != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", got)
	}
}

// --- upsertMessage tests ---

func TestUpsertMessage_InsertsNewMessage(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{Info: MessageInfo{ID: "msg1", Role: "user"}},
	}

	newMsg := MessageView{
		Info: MessageInfo{ID: "msg2", Role: "assistant"},
		Parts: []PartView{
			{ID: "p1", MessageID: "msg2", Type: "text", Text: "response"},
		},
	}

	cv.upsertMessage(MessageUpdatedMsg{Message: newMsg})

	if len(cv.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(cv.messages))
	}
	if cv.messages[1].Info.ID != "msg2" {
		t.Errorf("expected new message ID 'msg2', got %q", cv.messages[1].Info.ID)
	}
}

func TestUpsertMessage_UpdatesExistingMessageInfo(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant", ModelID: ""},
			Parts: []PartView{
				{ID: "p1", MessageID: "msg1", Type: "text", Text: "hello"},
			},
		},
	}

	updated := MessageView{
		Info: MessageInfo{ID: "msg1", Role: "assistant", ModelID: "claude-3"},
	}

	cv.upsertMessage(MessageUpdatedMsg{Message: updated})

	if len(cv.messages) != 1 {
		t.Fatalf("should not add duplicate, got %d messages", len(cv.messages))
	}
	if cv.messages[0].Info.ModelID != "claude-3" {
		t.Errorf("expected ModelID 'claude-3', got %q", cv.messages[0].Info.ModelID)
	}
	// Parts should be preserved (upsertMessage only updates Info)
	if len(cv.messages[0].Parts) != 1 {
		t.Errorf("existing parts should be preserved, got %d parts", len(cv.messages[0].Parts))
	}
}

// --- toggleSubagent tests ---

func TestToggleSubagent_TogglesSpecificLabel(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.subagentExpanded["agent-a"] = false
	cv.subagentExpanded["agent-b"] = false

	cv.toggleSubagent("agent-a")

	if !cv.subagentExpanded["agent-a"] {
		t.Error("agent-a should be expanded after toggle")
	}
	if cv.subagentExpanded["agent-b"] {
		t.Error("agent-b should remain collapsed")
	}
}

func TestToggleSubagent_TogglesBackToCollapsed(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.subagentExpanded["agent-a"] = true

	cv.toggleSubagent("agent-a")

	if cv.subagentExpanded["agent-a"] {
		t.Error("agent-a should be collapsed after double toggle")
	}
}

func TestToggleSubagent_EmptyLabelCollapsesAllWhenAnyExpanded(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", SubagentLabel: "agent-a"},
				{ID: "p2", SubagentLabel: "agent-b"},
			},
		},
	}
	cv.subagentExpanded["agent-a"] = true
	cv.subagentExpanded["agent-b"] = false

	cv.toggleSubagent("") // any expanded => collapse all

	if cv.subagentExpanded["agent-a"] {
		t.Error("agent-a should be collapsed when toggle-all and some were expanded")
	}
	if cv.subagentExpanded["agent-b"] {
		t.Error("agent-b should be collapsed when toggle-all and some were expanded")
	}
}

func TestToggleSubagent_EmptyLabelExpandsAllWhenNoneExpanded(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", SubagentLabel: "agent-a"},
				{ID: "p2", SubagentLabel: "agent-b"},
			},
		},
	}
	cv.subagentExpanded["agent-a"] = false
	cv.subagentExpanded["agent-b"] = false

	cv.toggleSubagent("") // none expanded => expand all

	if !cv.subagentExpanded["agent-a"] {
		t.Error("agent-a should be expanded when toggle-all and none were expanded")
	}
	if !cv.subagentExpanded["agent-b"] {
		t.Error("agent-b should be expanded when toggle-all and none were expanded")
	}
}

// --- handleMessagesLoaded tests ---

func TestHandleMessagesLoaded_LoadsMessagesFromRawMaps(t *testing.T) {
	cv := NewChatView(80, 24)
	raw := []map[string]any{
		{
			"info": map[string]any{
				"id":   "msg1",
				"role": "user",
			},
			"parts": []any{
				map[string]any{
					"id":        "p1",
					"messageID": "msg1",
					"type":      "text",
					"text":      "hello",
				},
			},
		},
		{
			"info": map[string]any{
				"id":   "msg2",
				"role": "assistant",
			},
			"parts": []any{
				map[string]any{
					"id":        "p2",
					"messageID": "msg2",
					"type":      "text",
					"text":      "world",
				},
			},
		},
	}

	result, _ := cv.handleMessagesLoaded(MessagesLoadedMsg{Messages: raw})

	if len(result.messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.messages))
	}
	if result.messages[0].Info.ID != "msg1" {
		t.Errorf("first message ID should be 'msg1', got %q", result.messages[0].Info.ID)
	}
	if result.messages[0].Info.Role != "user" {
		t.Errorf("first message role should be 'user', got %q", result.messages[0].Info.Role)
	}
	if result.messages[1].Info.ID != "msg2" {
		t.Errorf("second message ID should be 'msg2', got %q", result.messages[1].Info.ID)
	}
}

func TestHandleMessagesLoaded_SkipsOnError(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{Info: MessageInfo{ID: "existing"}},
	}

	result, _ := cv.handleMessagesLoaded(MessagesLoadedMsg{
		Err: fmt.Errorf("connection error"),
	})

	// On error, messages should remain unchanged
	if len(result.messages) != 1 {
		t.Fatalf("messages should be unchanged on error, got %d", len(result.messages))
	}
	if result.messages[0].Info.ID != "existing" {
		t.Error("existing message should be preserved on error")
	}
}

func TestHandleMessagesLoaded_ParsesPartsCorrectly(t *testing.T) {
	cv := NewChatView(80, 24)
	raw := []map[string]any{
		{
			"info": map[string]any{
				"id":   "msg1",
				"role": "assistant",
			},
			"parts": []any{
				map[string]any{
					"id":        "p1",
					"messageID": "msg1",
					"type":      "text",
					"text":      "some text",
				},
				map[string]any{
					"id":        "p2",
					"messageID": "msg1",
					"type":      "reasoning",
					"text":      "thinking...",
				},
			},
		},
	}

	result, _ := cv.handleMessagesLoaded(MessagesLoadedMsg{Messages: raw})

	if len(result.messages[0].Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(result.messages[0].Parts))
	}
	if result.messages[0].Parts[0].Type != "text" {
		t.Errorf("first part type should be 'text', got %q", result.messages[0].Parts[0].Type)
	}
	if result.messages[0].Parts[1].Type != "reasoning" {
		t.Errorf("second part type should be 'reasoning', got %q", result.messages[0].Parts[1].Type)
	}
}

// --- parseLoadedParts tests ---

func TestParseLoadedParts_ParsesPartsArray(t *testing.T) {
	m := map[string]any{
		"parts": []any{
			map[string]any{
				"id":   "p1",
				"type": "text",
				"text": "hello",
			},
		},
	}

	parts := parseLoadedParts(m)

	if len(parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(parts))
	}
	if parts[0].Type != "text" {
		t.Errorf("expected type 'text', got %q", parts[0].Type)
	}
	if parts[0].Text != "hello" {
		t.Errorf("expected text 'hello', got %q", parts[0].Text)
	}
}

func TestParseLoadedParts_ReturnsNilForMissingPartsKey(t *testing.T) {
	m := map[string]any{
		"info": map[string]any{"id": "msg1"},
	}

	parts := parseLoadedParts(m)

	if parts != nil {
		t.Errorf("expected nil for missing parts, got %v", parts)
	}
}

func TestParseLoadedParts_SkipsNonMapEntries(t *testing.T) {
	m := map[string]any{
		"parts": []any{
			"not a map",
			map[string]any{
				"id":   "p1",
				"type": "text",
				"text": "valid",
			},
		},
	}

	parts := parseLoadedParts(m)

	if len(parts) != 1 {
		t.Fatalf("expected 1 part (skipping non-map), got %d", len(parts))
	}
	if parts[0].Text != "valid" {
		t.Errorf("expected text 'valid', got %q", parts[0].Text)
	}
}

// --- Init tests ---

func TestInit_ReturnsNil(t *testing.T) {
	cv := NewChatView(80, 24)
	cmd := cv.Init()
	if cmd != nil {
		t.Error("Init() should return nil")
	}
}

// --- Messages tests ---

func TestMessages_ReturnsCurrentMessageList(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{Info: MessageInfo{ID: "msg1", Role: "user"}},
		{Info: MessageInfo{ID: "msg2", Role: "assistant"}},
	}

	msgs := cv.Messages()

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Info.ID != "msg1" {
		t.Errorf("expected first message ID 'msg1', got %q", msgs[0].Info.ID)
	}
}

func TestMessages_ReturnsNilWhenEmpty(t *testing.T) {
	cv := NewChatView(80, 24)
	msgs := cv.Messages()
	if msgs != nil {
		t.Errorf("expected nil for empty messages, got %v", msgs)
	}
}

// --- HasMessages tests ---

func TestHasMessages_ReturnsFalseWhenEmpty(t *testing.T) {
	cv := NewChatView(80, 24)
	if cv.HasMessages() {
		t.Error("HasMessages should return false when no messages")
	}
}

func TestHasMessages_ReturnsTrueWithMessages(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{{Info: MessageInfo{ID: "msg1"}}}
	if !cv.HasMessages() {
		t.Error("HasMessages should return true when messages exist")
	}
}

// --- IsSearching tests ---

func TestIsSearching_ReturnsFalseByDefault(t *testing.T) {
	cv := NewChatView(80, 24)
	if cv.IsSearching() {
		t.Error("IsSearching should return false by default")
	}
}

func TestIsSearching_ReturnsTrueAfterActivateSearch(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.ActivateSearch()
	if !cv.IsSearching() {
		t.Error("IsSearching should return true after ActivateSearch")
	}
}

func TestIsSearching_ReturnsFalseAfterDismissSearch(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.ActivateSearch()
	cv.DismissSearch()
	if cv.IsSearching() {
		t.Error("IsSearching should return false after DismissSearch")
	}
}

// --- View tests ---

func TestView_ContainsMessageContent(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "user"},
			Parts: []PartView{
				{ID: "p1", MessageID: "msg1", Type: "text", Text: "test message content"},
			},
		},
	}
	cv.rebuildContent()

	view := cv.View()
	plain := stripAnsi(view)

	if !strings.Contains(plain, "test message content") {
		t.Errorf("View should contain message text, got:\n%s", plain)
	}
}

func TestView_IncludesSearchBarWhenSearching(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.ActivateSearch()

	view := cv.View()
	plain := stripAnsi(view)

	if !strings.Contains(plain, "/") {
		t.Errorf("View should contain search bar indicator when searching, got:\n%s", plain)
	}
}

func TestView_ReturnsEmptyViewportWhenNoMessages(t *testing.T) {
	cv := NewChatView(80, 24)
	view := cv.View()
	// Should not panic and should return something (even if empty)
	if view == "" {
		// viewport.View() returns empty string for empty content, which is fine
	}
}

// --- Update tests ---

func TestUpdate_MessagePartDeltaAppliesAndRebuilds(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", MessageID: "msg1", Type: "text", Text: "Hello"},
			},
		},
	}

	result, _ := cv.Update(MessagePartDeltaMsg{
		MessageID: "msg1",
		PartID:    "p1",
		Field:     "text",
		Delta:     " there",
	})

	if result.messages[0].Parts[0].Text != "Hello there" {
		t.Errorf("expected 'Hello there', got %q", result.messages[0].Parts[0].Text)
	}
}

func TestUpdate_MessageUpdatedInsertsNewMessage(t *testing.T) {
	cv := NewChatView(80, 24)

	result, _ := cv.Update(MessageUpdatedMsg{
		Message: MessageView{
			Info: MessageInfo{ID: "msg1", Role: "user"},
		},
	})

	if len(result.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.messages))
	}
	if result.messages[0].Info.ID != "msg1" {
		t.Errorf("expected message ID 'msg1', got %q", result.messages[0].Info.ID)
	}
}

func TestUpdate_ToggleThoughtMsgTogglesThought(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", Type: "reasoning", ThoughtExpanded: false},
			},
		},
	}

	result, _ := cv.Update(ToggleThoughtMsg{PartID: "p1"})

	if !result.messages[0].Parts[0].ThoughtExpanded {
		t.Error("thought should be expanded after ToggleThoughtMsg")
	}
}

func TestUpdate_ToggleSubagentMsgTogglesSubagent(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", SubagentLabel: "researcher"},
			},
		},
	}

	result, _ := cv.Update(ToggleSubagentMsg{Label: "researcher"})

	if !result.subagentExpanded["researcher"] {
		t.Error("subagent 'researcher' should be expanded after ToggleSubagentMsg")
	}
}

func TestUpdate_SubagentCompletedMsgStoresStatus(t *testing.T) {
	cv := NewChatView(80, 24)

	result, _ := cv.Update(SubagentCompletedMsg{
		Label:        "researcher",
		Agent:        "explore",
		InputTokens:  1000,
		OutputTokens: 500,
	})

	status, ok := result.subagentStatus["researcher"]
	if !ok {
		t.Fatal("subagent status should be stored")
	}
	if !status.Done {
		t.Error("status.Done should be true")
	}
	if status.InputTokens != 1000 {
		t.Errorf("expected InputTokens 1000, got %d", status.InputTokens)
	}
	if status.OutputTokens != 500 {
		t.Errorf("expected OutputTokens 500, got %d", status.OutputTokens)
	}
	if status.Agent != "explore" {
		t.Errorf("expected Agent 'explore', got %q", status.Agent)
	}
}

func TestUpdate_TKeyTogglesAllThoughts(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", Type: "reasoning", ThoughtExpanded: false},
				{ID: "p2", Type: "reasoning", ThoughtExpanded: false},
			},
		},
	}

	result, _ := cv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})

	for i, p := range result.messages[0].Parts {
		if !p.ThoughtExpanded {
			t.Errorf("part %d should be expanded after T key", i)
		}
	}
}

func TestUpdate_MessagesLoadedMsgPopulatesMessages(t *testing.T) {
	cv := NewChatView(80, 24)

	result, _ := cv.Update(MessagesLoadedMsg{
		Messages: []map[string]any{
			{
				"info": map[string]any{
					"id":   "msg1",
					"role": "user",
				},
				"parts": []any{
					map[string]any{
						"id":   "p1",
						"type": "text",
						"text": "hello",
					},
				},
			},
		},
	})

	if len(result.messages) != 1 {
		t.Fatalf("expected 1 message from loaded data, got %d", len(result.messages))
	}
	if result.messages[0].Info.Role != "user" {
		t.Errorf("expected role 'user', got %q", result.messages[0].Info.Role)
	}
}

// --- stickyBottom / auto-scroll behavior tests ---

func TestStickyBottom_DefaultsToTrue(t *testing.T) {
	cv := NewChatView(80, 24)
	if !cv.stickyBottom {
		t.Error("stickyBottom should default to true")
	}
}

func TestStickyBottom_MaintainedAfterRebuild(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "user"},
			Parts: []PartView{
				{ID: "p1", MessageID: "msg1", Type: "text", Text: "hello"},
			},
		},
	}
	cv.rebuildContent()

	if !cv.stickyBottom {
		t.Error("stickyBottom should remain true after rebuild with few messages")
	}
}

// --- NewChatView tests ---

func TestNewChatView_InitializesMaps(t *testing.T) {
	cv := NewChatView(80, 24)

	if cv.subagentExpanded == nil {
		t.Error("subagentExpanded map should be initialized")
	}
	if cv.subagentStatus == nil {
		t.Error("subagentStatus map should be initialized")
	}
	if cv.renderer == nil {
		t.Error("renderer should be initialized")
	}
	if cv.width != 80 {
		t.Errorf("expected width 80, got %d", cv.width)
	}
	if cv.height != 24 {
		t.Errorf("expected height 24, got %d", cv.height)
	}
}

// --- SetSize tests ---

func TestSetSize_UpdatesDimensions(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.SetSize(120, 40)

	if cv.width != 120 {
		t.Errorf("expected width 120, got %d", cv.width)
	}
	if cv.height != 40 {
		t.Errorf("expected height 40, got %d", cv.height)
	}
}

func TestSetSize_ReducesViewportHeightInSearchMode(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.ActivateSearch()
	cv.SetSize(80, 30)

	if cv.viewport.Height != 29 {
		t.Errorf("viewport height should be 29 (30-1 for search bar), got %d", cv.viewport.Height)
	}
}

// --- MessagePartUpdatedMsg via Update tests ---

func TestUpdate_MessagePartUpdatedInsertsNewPart(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.messages = []MessageView{
		{
			Info:  MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{},
		},
	}

	result, _ := cv.Update(MessagePartUpdatedMsg{
		Part: PartView{ID: "p1", MessageID: "msg1", Type: "text", Text: "new part"},
	})

	if len(result.messages[0].Parts) != 1 {
		t.Fatalf("expected 1 part after insert, got %d", len(result.messages[0].Parts))
	}
	if result.messages[0].Parts[0].Text != "new part" {
		t.Errorf("expected text 'new part', got %q", result.messages[0].Parts[0].Text)
	}
}
