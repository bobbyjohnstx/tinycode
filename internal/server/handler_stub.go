package server

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/bobbyjohnstx/tinycode-go/internal/command"
	"github.com/bobbyjohnstx/tinycode-go/internal/config"
	"github.com/bobbyjohnstx/tinycode-go/internal/project"
	"github.com/bobbyjohnstx/tinycode-go/internal/skill"
)

func (s *Server) handleGlobalEventStream(w http.ResponseWriter, r *http.Request) {
	StreamEvents(r.Context(), w, s.deps.Bus, "")
}

func (s *Server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{})
}

func (s *Server) handleSessionInit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	respondJSON(w, http.StatusOK, map[string]any{
		"sessionID": id,
		"status":    "initialized",
	})
}

func (s *Server) handleSessionSummarize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	respondJSON(w, http.StatusAccepted, map[string]any{
		"sessionID": id,
		"status":    "summarizing",
	})
}

func (s *Server) handleSessionCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Command string `json:"command"`
		Args    string `json:"args,omitempty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s.deps.Bus.Publish("session.command", map[string]any{
		"sessionID": id,
		"command":   body.Command,
		"args":      body.Args,
	})
	respondJSON(w, http.StatusAccepted, map[string]any{
		"sessionID": id,
		"command":   body.Command,
	})
}

func (s *Server) handleSessionRevert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	respondJSON(w, http.StatusOK, map[string]any{
		"sessionID": id,
		"status":    "reverted",
	})
}

func (s *Server) handleSessionUnrevert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	respondJSON(w, http.StatusOK, map[string]any{
		"sessionID": id,
		"status":    "unreverted",
	})
}

func (s *Server) handleSessionChildren(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"children": []any{},
	})
}

func (s *Server) handleSessionTodo(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"todos": []any{},
	})
}

func (s *Server) handleSessionDiff(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"diff": "",
	})
}

func (s *Server) handleMessageDelete(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSessionPermissionReply(w http.ResponseWriter, r *http.Request) {
	permissionID := r.PathValue("permissionID")
	var body struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"permissionID": permissionID,
		"status":       "replied",
	})
}

func (s *Server) handleQuestionList(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"questions": []any{},
	})
}

func (s *Server) handleQuestionReply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	respondJSON(w, http.StatusOK, map[string]any{
		"questionID": id,
		"status":     "replied",
	})
}

func (s *Server) handleFileSearch(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"results": []any{},
	})
}

func (s *Server) handleFileFind(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"files": []any{},
	})
}

func (s *Server) handleProjectList(w http.ResponseWriter, r *http.Request) {
	p := project.FromDirectory(s.config.Directory)
	respondJSON(w, http.StatusOK, []*project.Info{p})
}

func (s *Server) handleProjectCurrent(w http.ResponseWriter, r *http.Request) {
	p := project.FromDirectory(s.config.Directory)
	respondJSON(w, http.StatusOK, p)
}

func (s *Server) handlePathGet(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config", "tinycode")
	stateDir := filepath.Join(home, ".local", "share", "tinycode")

	respondJSON(w, http.StatusOK, map[string]any{
		"home":      home,
		"state":     stateDir,
		"config":    configDir,
		"worktree":  s.config.Directory,
		"directory": s.config.Directory,
	})
}

func (s *Server) handleAgentList(w http.ResponseWriter, r *http.Request) {
	if s.deps.AgentRegistry != nil {
		agents := s.deps.AgentRegistry.List("")
		respondJSON(w, http.StatusOK, agents)
		return
	}
	respondJSON(w, http.StatusOK, []any{})
}

func (s *Server) handleSkillList(w http.ResponseWriter, r *http.Request) {
	configDir := config.ConfigDir()
	skills := skill.Discover(configDir, s.config.Directory)
	respondJSON(w, http.StatusOK, skills)
}

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

func (s *Server) handleVCSInfo(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"type":   "git",
		"branch": "",
	})
}

func (s *Server) handleVCSStatus(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"clean":   true,
		"changes": []any{},
	})
}

func (s *Server) handleVCSDiff(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"diff": "",
	})
}
