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
	var skillPaths []string
	if s.deps.Config != nil && s.deps.Config.Skills != nil {
		skillPaths = s.deps.Config.Skills.Paths
	}
	commands := command.DiscoverWithPaths(configDir, s.config.Directory, agentNames, skillPaths)
	respondJSON(w, http.StatusOK, commands)
}
