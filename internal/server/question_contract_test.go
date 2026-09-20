package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestQuestionList_ReturnsRawArray(t *testing.T) {
	srv, _ := testServer(t)

	srv.questionStore.Add(PendingQuestion{
		ID:        "q_1",
		SessionID: "ses_1",
		Question:  "Continue?",
		Options:   []string{"yes", "no"},
	})

	req := httptest.NewRequest("GET", "/question", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Must decode as JSON array, not {"questions": [...]}.
	var arr []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&arr); err != nil {
		t.Fatalf("expected JSON array, decode error: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 question, got %d", len(arr))
	}
	if arr[0]["id"] != "q_1" {
		t.Errorf("expected id q_1, got %v", arr[0]["id"])
	}
}

func TestQuestionList_EmptyReturnsEmptyArray(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/question", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != "[]" {
		t.Errorf("expected empty JSON array '[]', got %q", body)
	}
}

func TestQuestionList_SDKQuestionRequestShape(t *testing.T) {
	srv, _ := testServer(t)

	srv.questionStore.Add(PendingQuestion{
		ID:        "q_2",
		SessionID: "ses_2",
		Question:  "Pick a color",
		Options:   []string{"red", "blue"},
	})

	req := httptest.NewRequest("GET", "/question", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var arr []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&arr); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 item, got %d", len(arr))
	}

	item := arr[0]

	// Must have id, sessionID, questions fields.
	if item["id"] != "q_2" {
		t.Errorf("expected id q_2, got %v", item["id"])
	}
	if item["sessionID"] != "ses_2" {
		t.Errorf("expected sessionID ses_2, got %v", item["sessionID"])
	}

	// questions must be an array.
	questions, ok := item["questions"].([]any)
	if !ok {
		t.Fatalf("expected questions array, got %T", item["questions"])
	}
	if len(questions) != 1 {
		t.Fatalf("expected 1 question item, got %d", len(questions))
	}

	qi, ok := questions[0].(map[string]any)
	if !ok {
		t.Fatalf("expected question item to be object, got %T", questions[0])
	}
	if qi["question"] != "Pick a color" {
		t.Errorf("expected question text 'Pick a color', got %v", qi["question"])
	}

	// options must be [{label: "red"}, {label: "blue"}].
	opts, ok := qi["options"].([]any)
	if !ok {
		t.Fatalf("expected options array, got %T", qi["options"])
	}
	if len(opts) != 2 {
		t.Fatalf("expected 2 options, got %d", len(opts))
	}
	opt0, _ := opts[0].(map[string]any)
	if opt0["label"] != "red" {
		t.Errorf("expected first option label 'red', got %v", opt0["label"])
	}
}

func TestQuestionList_NoOptionsOmitted(t *testing.T) {
	srv, _ := testServer(t)

	srv.questionStore.Add(PendingQuestion{
		ID:        "q_3",
		SessionID: "ses_3",
		Question:  "What is your name?",
	})

	req := httptest.NewRequest("GET", "/question", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var arr []map[string]any
	json.NewDecoder(w.Body).Decode(&arr)

	qi := arr[0]["questions"].([]any)[0].(map[string]any)
	if _, hasOpts := qi["options"]; hasOpts {
		t.Error("expected options to be omitted when empty")
	}
}

func TestQuestionReply_PublishesQuestionReplied(t *testing.T) {
	srv, b := testServer(t)

	// Pre-populate the store so sessionID can be looked up.
	srv.questionStore.Add(PendingQuestion{
		ID:        "q_reply_1",
		SessionID: "ses_reply",
		Question:  "proceed?",
	})

	sub := b.Subscribe("question.replied")
	defer sub.Unsubscribe()

	body := `{"answer":"yes"}`
	req := httptest.NewRequest("POST", "/question/q_reply_1/reply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case evt := <-sub.C:
		props, ok := evt.Properties.(map[string]any)
		if !ok {
			t.Fatal("expected map properties on bus event")
		}
		// Verify event name is "question.replied" (not "question.reply").
		if evt.Type != "question.replied" {
			t.Errorf("expected event type question.replied, got %s", evt.Type)
		}
		// Verify fields: requestID, sessionID, answer.
		if props["requestID"] != "q_reply_1" {
			t.Errorf("expected requestID q_reply_1, got %v", props["requestID"])
		}
		if props["sessionID"] != "ses_reply" {
			t.Errorf("expected sessionID ses_reply, got %v", props["sessionID"])
		}
		if props["answer"] != "yes" {
			t.Errorf("expected answer 'yes', got %v", props["answer"])
		}
		// Verify old field name is NOT present.
		if _, has := props["questionID"]; has {
			t.Error("unexpected legacy field 'questionID' in event")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for question.replied bus event")
	}
}

func TestQuestionReply_OldEventNameNotPublished(t *testing.T) {
	srv, b := testServer(t)

	srv.questionStore.Add(PendingQuestion{
		ID:        "q_old",
		SessionID: "ses_old",
		Question:  "test?",
	})

	// Subscribe to the OLD event name.
	oldSub := b.Subscribe("question.reply")
	defer oldSub.Unsubscribe()

	body := `{"answer":"no"}`
	req := httptest.NewRequest("POST", "/question/q_old/reply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Old event name should NOT fire.
	select {
	case <-oldSub.C:
		t.Error("should not receive event on old 'question.reply' topic")
	case <-time.After(100 * time.Millisecond):
		// Expected: no event on old topic.
	}
}

func TestQuestionReply_RemovesFromStore(t *testing.T) {
	srv, _ := testServer(t)

	srv.questionStore.Add(PendingQuestion{
		ID:        "q_rm",
		SessionID: "ses_rm",
		Question:  "remove me?",
	})

	body := `{"answer":"done"}`
	req := httptest.NewRequest("POST", "/question/q_rm/reply", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if len(srv.questionStore.List()) != 0 {
		t.Error("expected question to be removed from store after reply")
	}
}

func TestQuestionStore_Get(t *testing.T) {
	qs := NewQuestionStore()
	qs.Add(PendingQuestion{ID: "q1", SessionID: "s1", Question: "test?"})

	q, ok := qs.Get("q1")
	if !ok {
		t.Fatal("expected Get to find q1")
	}
	if q.SessionID != "s1" {
		t.Errorf("expected sessionID s1, got %s", q.SessionID)
	}

	_, ok = qs.Get("nonexistent")
	if ok {
		t.Error("expected Get to return false for missing key")
	}
}
