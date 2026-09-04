package server

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bobbyjohnstx/tinycode-go/internal/project"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
	"github.com/bobbyjohnstx/tinycode-go/internal/vcs"
)

func (s *Server) handleGlobalEventStream(w http.ResponseWriter, r *http.Request) {
	StreamEvents(r.Context(), w, s.deps.Bus, "")
}

func (s *Server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	status := s.sessionManager.Status()
	respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleSessionInit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	info, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	s.deps.Bus.Publish("session.initialized", map[string]any{
		"sessionID": id,
	})

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionSummarize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	info, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	s.deps.Bus.Publish("session.summarize", map[string]any{
		"sessionID": id,
	})

	respondJSON(w, http.StatusAccepted, info)
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
	store := s.sessionStore()

	info, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	s.deps.Bus.Publish("session.revert", map[string]any{
		"sessionID": id,
	})

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionUnrevert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	info, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	s.deps.Bus.Publish("session.unrevert", map[string]any{
		"sessionID": id,
	})

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionChildren(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	children, err := store.Children(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if children == nil {
		children = []session.Info{}
	}

	respondJSON(w, http.StatusOK, children)
}

func (s *Server) handleSessionTodo(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"todos": []any{},
	})
}

func (s *Server) handleSessionDiff(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"diff":    "",
		"files":   []any{},
		"summary": map[string]any{"additions": 0, "deletions": 0, "files": 0},
	})
}

func (s *Server) handleMessageDelete(w http.ResponseWriter, r *http.Request) {
	messageID := r.PathValue("messageID")

	ms := s.messageStore()
	if err := ms.DeleteByID(messageID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ps := s.partStore()
	_ = ps.DeleteByMessage(messageID)

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

	s.permissionStore.Remove(permissionID)

	s.deps.Bus.Publish("permission.reply", map[string]any{
		"permissionID": permissionID,
		"action":       body.Action,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"permissionID": permissionID,
		"status":       "replied",
	})
}

func (s *Server) handleQuestionList(w http.ResponseWriter, r *http.Request) {
	questions := s.questionStore.List()
	respondJSON(w, http.StatusOK, map[string]any{
		"questions": questions,
	})
}

func (s *Server) handleQuestionReply(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Answer string `json:"answer"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.questionStore.Remove(id)

	s.deps.Bus.Publish("question.reply", map[string]any{
		"questionID": id,
		"answer":     body.Answer,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"questionID": id,
		"status":     "replied",
	})
}

func (s *Server) handleFileSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		respondJSON(w, http.StatusOK, map[string]any{
			"results": []any{},
		})
		return
	}

	type searchResult struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Content string `json:"content"`
	}

	var results []searchResult
	const maxResults = 100

	_ = filepath.Walk(s.config.Directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if len(results) >= maxResults {
			return filepath.SkipAll
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == ".tinycode" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 1<<20 {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		rel, _ := filepath.Rel(s.config.Directory, path)
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if strings.Contains(line, query) {
				results = append(results, searchResult{
					File:    rel,
					Line:    lineNum,
					Content: line,
				})
				if len(results) >= maxResults {
					break
				}
			}
		}
		return nil
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"results": results,
	})
}

func (s *Server) handleFileFind(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		respondJSON(w, http.StatusOK, map[string]any{
			"files": []string{},
		})
		return
	}

	var files []string
	const maxFiles = 200

	_ = filepath.Walk(s.config.Directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if len(files) >= maxFiles {
			return filepath.SkipAll
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == ".tinycode" {
				return filepath.SkipDir
			}
			return nil
		}

		name := info.Name()
		matched, _ := filepath.Match(query, name)
		if matched || strings.Contains(name, query) {
			rel, _ := filepath.Rel(s.config.Directory, path)
			files = append(files, rel)
		}
		return nil
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"files": files,
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
	agents := []map[string]any{
		{
			"name":        "build",
			"description": "Full tool access for implementation work",
			"mode":        "primary",
			"native":      true,
			"permission":  map[string]any{"allow": []string{"*"}, "deny": []string{}},
		},
		{
			"name":        "plan",
			"description": "Read-only planning and analysis",
			"mode":        "primary",
			"native":      true,
			"permission":  map[string]any{"allow": []string{"read", "glob", "grep", "bash"}, "deny": []string{}},
		},
		{
			"name":        "architect",
			"description": "Architecture analysis and design guidance",
			"mode":        "subagent",
			"native":      true,
			"permission":  map[string]any{"allow": []string{"read", "glob", "grep", "bash"}, "deny": []string{}},
		},
		{
			"name":        "debugger",
			"description": "Root-cause analysis and debugging",
			"mode":        "subagent",
			"native":      true,
			"permission":  map[string]any{"allow": []string{"*"}, "deny": []string{}},
		},
	}
	respondJSON(w, http.StatusOK, agents)
}

func (s *Server) handleSkillList(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, []any{})
}

func (s *Server) handleCommandList(w http.ResponseWriter, r *http.Request) {
	commands := []map[string]any{
		{
			"name":        "init",
			"description": "Guided project setup",
			"source":      "command",
			"template":    "Initialize this project for AI-assisted development.",
			"hints":       []string{},
		},
		{
			"name":        "review",
			"description": "Review changes — /review [commit|branch|pr]",
			"source":      "command",
			"template":    "Review the recent changes in this project.",
			"subtask":     true,
			"hints":       []string{"$1"},
		},
		{
			"name":        "ask",
			"description": "Ask an agent — /ask <agent> <prompt>",
			"source":      "command",
			"template":    "$2",
			"subtask":     true,
			"hints":       []string{"$1", "$2"},
		},
	}
	respondJSON(w, http.StatusOK, commands)
}

func (s *Server) handleVCSInfo(w http.ResponseWriter, r *http.Request) {
	info, err := vcs.GitInfo(s.config.Directory)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"type":   "git",
			"branch": "",
			"remote": "",
		})
		return
	}
	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleVCSStatus(w http.ResponseWriter, r *http.Request) {
	status, err := vcs.GitStatus(s.config.Directory)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"clean":   true,
			"changes": []any{},
		})
		return
	}
	respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleVCSDiff(w http.ResponseWriter, r *http.Request) {
	diff, err := vcs.GitDiff(s.config.Directory)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"diff": "",
		})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"diff": diff,
	})
}

func (s *Server) handleMCPStatus(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{})
}
