package server

import (
	"net/http"

	"github.com/bobbyjohnstx/tinycode/internal/command"
	"github.com/bobbyjohnstx/tinycode/internal/config"
)

func (s *Server) handleCommandList(w http.ResponseWriter, r *http.Request) {
	var agentNames []string
	if s.deps.AgentRegistry != nil {
		for _, a := range s.deps.AgentRegistry.List("") {
			agentNames = append(agentNames, a.Name)
		}
	}

	configDir := config.ConfigDir()
	commands := command.Discover(configDir, s.config.Directory, agentNames)
	respondJSON(w, http.StatusOK, commands)
}
