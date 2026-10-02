package server

import (
	"net/http"

	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/skill"
)

func (s *Server) handleSkillList(w http.ResponseWriter, r *http.Request) {
	configDir := config.ConfigDir()
	skills := skill.Discover(configDir, s.config.Directory)
	respondJSON(w, http.StatusOK, skills)
}
