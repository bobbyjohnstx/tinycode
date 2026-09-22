package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
)

func newSearchableChatView(msgs []MessageView) ChatView {
	cv := NewChatView(80, 24)
	cv.messages = msgs
	cv.rebuildContent()
	return cv
}

func TestFindMatchesCaseInsensitive(t *testing.T) {
	cv := newSearchableChatView([]MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "user"},
			Parts: []PartView{
				{ID: "p1", Type: "text", Text: "Hello World hello"},
			},
		},
	})
	cv.searchInput.SetValue("hello")
	cv.findMatches()

	if len(cv.searchMatches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(cv.searchMatches))
	}
	if cv.searchMatches[0].offset != 0 {
		t.Errorf("first match offset: want 0, got %d", cv.searchMatches[0].offset)
	}
	if cv.searchMatches[1].offset != 12 {
		t.Errorf("second match offset: want 12, got %d", cv.searchMatches[1].offset)
	}
}

func TestFindMatchesAcrossMessages(t *testing.T) {
	cv := newSearchableChatView([]MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "user"},
			Parts: []PartView{
				{ID: "p1", Type: "text", Text: "first match here"},
			},
		},
		{
			Info: MessageInfo{ID: "msg2", Role: "assistant"},
			Parts: []PartView{
				{ID: "p2", Type: "text", Text: "second match here"},
			},
		},
	})
	cv.searchInput.SetValue("match")
	cv.findMatches()

	if len(cv.searchMatches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(cv.searchMatches))
	}
	if cv.searchMatches[0].msgIdx != 0 {
		t.Errorf("first match msgIdx: want 0, got %d", cv.searchMatches[0].msgIdx)
	}
	if cv.searchMatches[1].msgIdx != 1 {
		t.Errorf("second match msgIdx: want 1, got %d", cv.searchMatches[1].msgIdx)
	}
}

func TestFindMatchesEmptyQuery(t *testing.T) {
	cv := newSearchableChatView([]MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "user"},
			Parts: []PartView{
				{ID: "p1", Type: "text", Text: "some text"},
			},
		},
	})
	cv.searchInput.SetValue("")
	cv.findMatches()

	if len(cv.searchMatches) != 0 {
		t.Fatalf("expected 0 matches for empty query, got %d", len(cv.searchMatches))
	}
}

func TestFindMatchesSkipsNonTextParts(t *testing.T) {
	cv := newSearchableChatView([]MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", Type: "tool-call", Text: "hello", ToolName: "bash"},
				{ID: "p2", Type: "text", Text: "hello world"},
			},
		},
	})
	cv.searchInput.SetValue("hello")
	cv.findMatches()

	if len(cv.searchMatches) != 1 {
		t.Fatalf("expected 1 match (tool-call skipped), got %d", len(cv.searchMatches))
	}
	if cv.searchMatches[0].partIdx != 1 {
		t.Errorf("match partIdx: want 1, got %d", cv.searchMatches[0].partIdx)
	}
}

func TestFindMatchesIncludesReasoning(t *testing.T) {
	cv := newSearchableChatView([]MessageView{
		{
			Info: MessageInfo{ID: "msg1", Role: "assistant"},
			Parts: []PartView{
				{ID: "p1", Type: "reasoning", Text: "thinking about hello"},
				{ID: "p2", Type: "text", Text: "the answer"},
			},
		},
	})
	cv.searchInput.SetValue("hello")
	cv.findMatches()

	if len(cv.searchMatches) != 1 {
		t.Fatalf("expected 1 match in reasoning, got %d", len(cv.searchMatches))
	}
	if cv.searchMatches[0].partIdx != 0 {
		t.Errorf("match partIdx: want 0 (reasoning), got %d", cv.searchMatches[0].partIdx)
	}
}

func TestSearchNavigationWraps(t *testing.T) {
	cv := ChatView{
		searchMatches: []searchMatch{
			{msgIdx: 0}, {msgIdx: 1}, {msgIdx: 2},
		},
		searchCurrent: 0,
		searchInput:   textinput.New(),
	}

	// Next from last wraps to first.
	cv.searchCurrent = 2
	cv.nextMatch()
	if cv.searchCurrent != 0 {
		t.Errorf("next from last: want 0, got %d", cv.searchCurrent)
	}

	// Prev from first wraps to last.
	cv.prevMatch()
	if cv.searchCurrent != 2 {
		t.Errorf("prev from first: want 2, got %d", cv.searchCurrent)
	}
}

func TestSearchNavigationNoMatches(t *testing.T) {
	cv := ChatView{
		searchMatches: nil,
		searchCurrent: 0,
		searchInput:   textinput.New(),
	}

	// Should not panic with no matches.
	cv.nextMatch()
	cv.prevMatch()

	if cv.searchCurrent != 0 {
		t.Errorf("searchCurrent should remain 0 with no matches, got %d", cv.searchCurrent)
	}
}

func TestActivateDismissSearch(t *testing.T) {
	cv := NewChatView(80, 24)

	cv.ActivateSearch()
	if !cv.searchMode {
		t.Error("expected searchMode true after ActivateSearch")
	}
	if cv.viewport.Height != 23 {
		t.Errorf("viewport height during search: want 23, got %d", cv.viewport.Height)
	}

	cv.DismissSearch()
	if cv.searchMode {
		t.Error("expected searchMode false after DismissSearch")
	}
	if cv.viewport.Height != 24 {
		t.Errorf("viewport height after dismiss: want 24, got %d", cv.viewport.Height)
	}
}

func TestSetSizePreservesSearchViewportHeight(t *testing.T) {
	cv := NewChatView(80, 24)
	cv.ActivateSearch()

	cv.SetSize(120, 40)
	if cv.viewport.Height != 39 {
		t.Errorf("viewport height after resize in search mode: want 39, got %d", cv.viewport.Height)
	}

	cv.DismissSearch()
	cv.SetSize(120, 40)
	if cv.viewport.Height != 40 {
		t.Errorf("viewport height after resize without search: want 40, got %d", cv.viewport.Height)
	}
}

func TestMsgLineStartsTracked(t *testing.T) {
	cv := newSearchableChatView([]MessageView{
		{
			Info:  MessageInfo{ID: "msg1", Role: "user"},
			Parts: []PartView{{ID: "p1", Type: "text", Text: "line one"}},
		},
		{
			Info:  MessageInfo{ID: "msg2", Role: "assistant"},
			Parts: []PartView{{ID: "p2", Type: "text", Text: "line two"}},
		},
	})

	if len(cv.msgLineStarts) != 2 {
		t.Fatalf("expected 2 msgLineStarts entries, got %d", len(cv.msgLineStarts))
	}
	if cv.msgLineStarts[0] != 0 {
		t.Errorf("first message should start at line 0, got %d", cv.msgLineStarts[0])
	}
	if cv.msgLineStarts[1] <= cv.msgLineStarts[0] {
		t.Error("second message should start after the first")
	}
}
