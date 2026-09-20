package tui

import (
	"testing"
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
