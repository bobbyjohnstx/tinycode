package server

import (
	"net/http"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/skill"
)

func (s *Server) handleSkillList(w http.ResponseWriter, r *http.Request) {
	configDir := config.ConfigDir()
	var skillPaths []string
	if s.deps.Config != nil && s.deps.Config.Skills != nil {
		skillPaths = s.deps.Config.Skills.Paths
	}
	skills := skill.DiscoverWithPaths(configDir, s.config.Directory, skillPaths)
	respondJSON(w, http.StatusOK, skills)
}
