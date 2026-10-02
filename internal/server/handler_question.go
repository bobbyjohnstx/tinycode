package server

import "net/http"

func (s *Server) handleQuestionList(w http.ResponseWriter, r *http.Request) {
	questions := s.questionStore.List()

	// Transform to SDK QuestionRequest format:
	// [{ id, sessionID, questions: [{ question, options: [{ label }] }] }]
	result := make([]map[string]any, 0, len(questions))
	for _, q := range questions {
		opts := make([]map[string]any, 0, len(q.Options))
		for _, o := range q.Options {
			opts = append(opts, map[string]any{"label": o})
		}
		item := map[string]any{
			"question": q.Question,
		}
		if len(opts) > 0 {
			item["options"] = opts
		}
		result = append(result, map[string]any{
			"id":        q.ID,
			"sessionID": q.SessionID,
			"questions": []map[string]any{item},
		})
	}
	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleQuestionReply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Answer string `json:"answer"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Look up sessionID before removing from store.
	var sessionID string
	if q, ok := s.questionStore.Get(id); ok {
		sessionID = q.SessionID
	}
	s.questionStore.Remove(id)

	s.deps.Bus.Publish("question.replied", map[string]any{
		"sessionID": sessionID,
		"requestID": id,
		"answer":    body.Answer,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"questionID": id,
		"status":     "replied",
	})
}
