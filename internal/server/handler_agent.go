package server

import (
	"net/http"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
)

func (s *Server) handleAgentList(w http.ResponseWriter, r *http.Request) {
	if s.deps.AgentRegistry != nil {
		var agents []agent.Info
		if r.URL.Query().Get("include") == "disabled" {
			agents = s.deps.AgentRegistry.ListAll("")
		} else {
			agents = s.deps.AgentRegistry.List("")
		}
		respondJSON(w, http.StatusOK, agents)
		return
	}
	respondJSON(w, http.StatusOK, []any{})
}
