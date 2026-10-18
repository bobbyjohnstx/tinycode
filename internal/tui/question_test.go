package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestQuestionPrompt_ShowQueuesSecondRequest(t *testing.T) {
	p := NewQuestionPrompt()
	p.Show(QuestionAskedMsg{ID: "q-1", Question: "First?"})
	p.Show(QuestionAskedMsg{ID: "q-2", Question: "Second?"})
	if p.VisibleID() != "q-1" {
		t.Fatalf("visible = %q, want q-1", p.VisibleID())
	}
	if !p.ShowNext() {
		t.Fatal("expected a queued request")
	}
	if p.VisibleID() != "q-2" {
		t.Fatalf("next = %q, want q-2", p.VisibleID())
	}
}

func TestQuestionPrompt_ShowNext_ShowsQueued(t *testing.T) {
	p := NewQuestionPrompt()
	p.Show(QuestionAskedMsg{ID: "q-1", Question: "One?"})
	p.Show(QuestionAskedMsg{ID: "q-2", Question: "Two?"})
	p.Hide()
	if !p.ShowNext() {
		t.Fatal("expected queued question")
	}
	if p.VisibleID() != "q-2" {
		t.Fatalf("visible = %q, want q-2", p.VisibleID())
	}
	if p.ShowNext() {
		t.Fatal("expected empty queue")
	}
	if p.IsVisible() {
		t.Fatal("expected hidden after empty ShowNext")
	}
}

func TestQuestionPrompt_Show_DedupesSameID(t *testing.T) {
	p := NewQuestionPrompt()
	p.Show(QuestionAskedMsg{ID: "q-1", Question: "First?"})
	p.Show(QuestionAskedMsg{ID: "q-1", Question: "Dup?"})
	p.Show(QuestionAskedMsg{ID: "q-2", Question: "Second?"})
	p.Show(QuestionAskedMsg{ID: "q-2", Question: "Dup2?"})
	if len(p.queue) != 1 {
		t.Fatalf("queue len = %d, want 1", len(p.queue))
	}
	if p.queue[0].ID != "q-2" {
		t.Fatalf("queued id = %q, want q-2", p.queue[0].ID)
	}
}

func TestQuestionPrompt_EscSetsRejected(t *testing.T) {
	p := NewQuestionPrompt()
	p.Show(QuestionAskedMsg{ID: "q-1", SessionID: "ses-1", Question: "Continue?", Options: []string{"yes", "no"}})
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if p.IsVisible() {
		t.Fatal("expected prompt closed after esc")
	}
	if cmd == nil {
		t.Fatal("expected dismiss cmd")
	}
	msg := cmd()
	dismissed, ok := msg.(QuestionDismissedMsg)
	if !ok {
		t.Fatalf("expected QuestionDismissedMsg, got %T", msg)
	}
	if !dismissed.Rejected {
		t.Fatal("expected Rejected=true")
	}
	if dismissed.ID != "q-1" {
		t.Errorf("id = %q, want q-1", dismissed.ID)
	}
}

func TestQuestionPrompt_removeQueued(t *testing.T) {
	p := NewQuestionPrompt()
	p.Show(QuestionAskedMsg{ID: "q-1", Question: "One?"})
	p.Show(QuestionAskedMsg{ID: "q-2", Question: "Two?"})
	p.Show(QuestionAskedMsg{ID: "q-3", Question: "Three?"})
	p.removeQueued("q-2")
	if len(p.queue) != 1 || p.queue[0].ID != "q-3" {
		t.Fatalf("queue = %+v, want [q-3]", p.queue)
	}
}

func TestQuestionPrompt_VisibleID_WhenHidden(t *testing.T) {
	p := NewQuestionPrompt()
	if id := p.VisibleID(); id != "" {
		t.Errorf("expected empty VisibleID when hidden, got %q", id)
	}
}

func TestQuestionPrompt_VisibleID_WhenShown(t *testing.T) {
	p := NewQuestionPrompt()
	p.Show(QuestionAskedMsg{ID: "q1", Question: "What?"})
	if id := p.VisibleID(); id != "q1" {
		t.Errorf("expected q1, got %q", id)
	}
}

func TestQuestionPrompt_FocusCmd_WhenHidden(t *testing.T) {
	p := NewQuestionPrompt()
	if cmd := p.FocusCmd(); cmd != nil {
		t.Error("expected nil cmd when hidden")
	}
}

func TestQuestionPrompt_FocusCmd_WithOptions(t *testing.T) {
	p := NewQuestionPrompt()
	p.Show(QuestionAskedMsg{
		ID:       "q2",
		Question: "Pick one",
		Options:  []string{"A", "B"},
	})
	if cmd := p.FocusCmd(); cmd != nil {
		t.Error("expected nil cmd for options-based question (no text input to focus)")
	}
}
