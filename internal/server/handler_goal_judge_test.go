package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestGoalJudge_ReturnsDoneVerdict(t *testing.T) {
	h := newTestHarness(t, []mockScenario{
		{textResponse: `{"verdict":"done","reason":"Tests pass","next_step":""}`},
	})
	sessionID := h.createSession("Goal Judge Test", "build")

	body := `{"objective":"all tests pass"}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/goal-judge", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("goal-judge request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result GoalJudgeResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.Verdict != "done" {
		t.Errorf("expected verdict 'done', got %q", result.Verdict)
	}
	if result.Reason != "Tests pass" {
		t.Errorf("expected reason 'Tests pass', got %q", result.Reason)
	}
}

func TestGoalJudge_ContinueVerdictWithNextStep(t *testing.T) {
	h := newTestHarness(t, []mockScenario{
		{textResponse: `{"verdict":"continue","reason":"Still working","next_step":"Run the tests"}`},
	})
	sessionID := h.createSession("Goal Judge Continue", "build")

	body := `{"objective":"fix the bug"}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/goal-judge", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("goal-judge request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result GoalJudgeResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.Verdict != "continue" {
		t.Errorf("expected verdict 'continue', got %q", result.Verdict)
	}
	if result.NextStep != "Run the tests" {
		t.Errorf("expected next_step 'Run the tests', got %q", result.NextStep)
	}
}

func TestGoalJudge_MalformedJSONReturnsContinue(t *testing.T) {
	h := newTestHarness(t, []mockScenario{
		{textResponse: "This is not JSON at all"},
	})
	sessionID := h.createSession("Goal Judge Malformed", "build")

	body := `{"objective":"do the thing"}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/goal-judge", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("goal-judge request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result GoalJudgeResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if result.Verdict != "continue" {
		t.Errorf("expected verdict 'continue' for malformed JSON, got %q", result.Verdict)
	}
}

func TestGoalJudge_EmptyObjectiveReturnsError(t *testing.T) {
	h := newTestHarness(t, nil)
	sessionID := h.createSession("Goal Judge Empty", "build")

	body := `{"objective":""}`
	resp, err := http.Post(h.baseURL()+"/session/"+sessionID+"/goal-judge", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("goal-judge request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGoalJudge_InvalidSessionReturnsNotFound(t *testing.T) {
	h := newTestHarness(t, nil)

	body := `{"objective":"test"}`
	resp, err := http.Post(h.baseURL()+"/session/nonexistent/goal-judge", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("goal-judge request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestParseGoalJudgeResponse_ValidJSON(t *testing.T) {
	result := parseGoalJudgeResponse(`{"verdict":"done","reason":"All good","next_step":""}`)
	if result.Verdict != "done" {
		t.Errorf("verdict = %q, want 'done'", result.Verdict)
	}
	if result.Reason != "All good" {
		t.Errorf("reason = %q, want 'All good'", result.Reason)
	}
}

func TestParseGoalJudgeResponse_MarkdownFences(t *testing.T) {
	result := parseGoalJudgeResponse("```json\n{\"verdict\":\"continue\",\"reason\":\"working\",\"next_step\":\"keep going\"}\n```")
	if result.Verdict != "continue" {
		t.Errorf("verdict = %q, want 'continue'", result.Verdict)
	}
	if result.NextStep != "keep going" {
		t.Errorf("next_step = %q, want 'keep going'", result.NextStep)
	}
}

func TestParseGoalJudgeResponse_InvalidJSON(t *testing.T) {
	result := parseGoalJudgeResponse("not json")
	if result.Verdict != "continue" {
		t.Errorf("verdict = %q, want 'continue' for malformed", result.Verdict)
	}
	if result.NextStep == "" {
		t.Error("expected non-empty next_step for malformed JSON")
	}
}

func TestParseGoalJudgeResponse_InvalidVerdict(t *testing.T) {
	result := parseGoalJudgeResponse(`{"verdict":"unknown","reason":"hmm","next_step":"do stuff"}`)
	if result.Verdict != "continue" {
		t.Errorf("verdict = %q, want 'continue' for invalid verdict", result.Verdict)
	}
}

func TestParseGoalJudgeResponse_EmptyNextStepOnContinue(t *testing.T) {
	result := parseGoalJudgeResponse(`{"verdict":"continue","reason":"in progress","next_step":""}`)
	if result.NextStep == "" {
		t.Error("expected non-empty next_step when verdict is continue and next_step is empty")
	}
}

func TestBuildGoalJudgeContext_Empty(t *testing.T) {
	result := buildGoalJudgeContext(nil)
	if result != "" {
		t.Errorf("expected empty string for nil messages, got %q", result)
	}
}
