package server

import (
	"bufio"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/command"
	"github.com/bobbyjohnstx/tinycode-go/internal/config"
	"github.com/bobbyjohnstx/tinycode-go/internal/project"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
	"github.com/bobbyjohnstx/tinycode-go/internal/skill"
	"github.com/bobbyjohnstx/tinycode-go/internal/vcs"
)

func (s *Server) handleGlobalEventStream(w http.ResponseWriter, r *http.Request) {
	StreamGlobalEvents(r.Context(), w, s.deps.Bus, s.config.Directory)
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
	if err := decodeJSON(w, r, &body); err != nil {
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
	sessionID := r.PathValue("id")
	ms := s.messageStore()
	messages, err := ms.List(sessionID)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{"todos": []any{}})
		return
	}

	type todoItem struct {
		Text      string `json:"text"`
		MessageID string `json:"messageID"`
		Line      int    `json:"line"`
	}

	var todos []todoItem
	for _, msg := range messages {
		for _, part := range msg.Parts {
			if part.Type != session.PartText || part.Text == "" {
				continue
			}
			lines := strings.Split(part.Text, "\n")
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				upper := strings.ToUpper(trimmed)
				if !strings.Contains(upper, "TODO") && !strings.Contains(upper, "FIXME") {
					continue
				}
				todos = append(todos, todoItem{
					Text:      trimmed,
					MessageID: msg.ID,
					Line:      i + 1,
				})
			}
		}
	}

	if todos == nil {
		todos = []todoItem{}
	}
	respondJSON(w, http.StatusOK, map[string]any{"todos": todos})
}

func (s *Server) handleSessionDiff(w http.ResponseWriter, r *http.Request) {
	diff, err := vcs.GitDiff(s.config.Directory)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"diff":    "",
			"files":   []any{},
			"summary": map[string]any{"additions": 0, "deletions": 0, "files": 0},
		})
		return
	}

	files, additions, deletions := parseDiffStats(diff)
	respondJSON(w, http.StatusOK, map[string]any{
		"diff":  diff,
		"files": files,
		"summary": map[string]any{
			"additions": additions,
			"deletions": deletions,
			"files":     len(files),
		},
	})
}

func parseDiffStats(diff string) (files []string, additions, deletions int) {
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				file := strings.TrimPrefix(parts[3], "b/")
				files = append(files, file)
			}
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			deletions++
		}
	}
	if files == nil {
		files = []string{}
	}
	return
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
		Action   string `json:"action"`
		Reply    string `json:"reply"`
		Response string `json:"response"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// The SDK sends "reply" or "response" (once/always/reject); normalize to "action".
	action := body.Action
	if action == "" {
		action = body.Reply
	}
	if action == "" {
		action = body.Response
	}

	// Normalize to TS-contract Reply type
	reply := action
	switch reply {
	case "allow":
		reply = "once"
	}

	sessionID := r.PathValue("sessionID")

	s.permissionStore.Remove(permissionID)

	s.deps.Bus.Publish("permission.replied", map[string]any{
		"sessionID": sessionID,
		"requestID": permissionID,
		"reply":     reply,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"permissionID": permissionID,
		"status":       "replied",
	})
}

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
			if !strings.Contains(line, query) {
				continue
			}
			results = append(results, searchResult{
				File:    rel,
				Line:    lineNum,
				Content: line,
			})
			if len(results) >= maxResults {
				break
			}
		}
		return nil
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"results": results,
	})
}

func (s *Server) handleFileFind(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	if query == "" {
		query = r.URL.Query().Get("q")
	}
	if query == "" {
		respondJSON(w, http.StatusOK, []string{})
		return
	}

	dir := requestDirectory(r, s.config.Directory)

	typeFilter := r.URL.Query().Get("type")

	limitStr := r.URL.Query().Get("limit")
	maxFiles := 200
	if limitStr != "" {
		if n := 0; len(limitStr) > 0 {
			for _, c := range limitStr {
				if c >= '0' && c <= '9' {
					n = n*10 + int(c-'0')
				}
			}
			if n > 0 {
				maxFiles = n
			}
		}
	}

	var files []string

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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
			if typeFilter == "directory" {
				if strings.Contains(strings.ToLower(name), strings.ToLower(query)) {
					files = append(files, path)
				}
			}
			return nil
		}

		if typeFilter == "directory" {
			return nil
		}

		name := info.Name()
		matched, _ := filepath.Match(query, name)
		if matched || strings.Contains(name, query) {
			rel, _ := filepath.Rel(dir, path)
			files = append(files, rel)
		}
		return nil
	})

	if files == nil {
		files = []string{}
	}
	respondJSON(w, http.StatusOK, files)
}

func (s *Server) handleProjectList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.deps.DB.Query(
		`SELECT id, worktree, vcs, time_created, time_initialized FROM project ORDER BY time_created`,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var projects []*project.Info
	for rows.Next() {
		var p project.Info
		var vcsVal sql.NullString
		var timeInit sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Worktree, &vcsVal, &p.Time.Created, &timeInit); err != nil {
			continue
		}
		if vcsVal.Valid {
			p.VCS = vcsVal.String
		}
		if timeInit.Valid {
			p.Time.Initialized = timeInit.Int64
		}
		projects = append(projects, &p)
	}
	if projects == nil {
		projects = []*project.Info{}
	}
	respondJSON(w, http.StatusOK, projects)
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

func (s *Server) handleGlobalDispose(w http.ResponseWriter, r *http.Request) {
	s.deps.Bus.Publish("global.disposed", map[string]any{
		"timestamp": time.Now().UnixMilli(),
	})
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleMessageGet(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	messageID := r.PathValue("messageID")

	ms := s.messageStore()
	ps := s.partStore()

	messages, err := ms.List(sessionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, m := range messages {
		if m.ID != messageID {
			continue
		}

		storedParts, _ := ps.ListByMessage(m.ID)
		var parts any
		if len(storedParts) > 0 {
			parts = storedParts
		} else if len(m.Parts) > 0 {
			parts = m.Parts
		} else {
			parts = []session.StoredPart{}
		}

		createdMs := m.CreatedAt.UnixMilli()
		info := map[string]any{
			"id":        m.ID,
			"sessionID": m.SessionID,
			"role":      string(m.Role),
			"time":      map[string]any{"created": createdMs},
		}
		if m.Role == session.RoleAssistant {
			info["time"] = map[string]any{"created": createdMs, "completed": createdMs}
		}
		if m.Model != "" {
			info["modelID"] = m.Model
		}

		respondJSON(w, http.StatusOK, map[string]any{
			"info":  info,
			"parts": parts,
		})
		return
	}

	respondError(w, http.StatusNotFound, "message not found")
}

func (s *Server) handleConfigProviders(w http.ResponseWriter, r *http.Request) {
	providers := s.deps.Registry.ListProviders()

	type providerSummary struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Source string `json:"source"`
		Models int    `json:"models"`
	}

	result := make([]providerSummary, 0, len(providers))
	for _, p := range providers {
		result = append(result, providerSummary{
			ID:     p.ID,
			Name:   p.Name,
			Source: p.Source,
			Models: len(p.Models),
		})
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleFileStatus(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleVCSDiffRaw(w http.ResponseWriter, r *http.Request) {
	diff, err := vcs.GitDiff(s.config.Directory)
	if err != nil {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(diff))
}

func (s *Server) handleLSP(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"status": "unavailable",
	})
}

func (s *Server) handleFormatter(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"status": "unavailable",
	})
}

func (s *Server) handleMCPStatus(w http.ResponseWriter, r *http.Request) {
	if s.deps.MCPService != nil {
		respondJSON(w, http.StatusOK, s.deps.MCPService.Status(r.Context()))
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{})
}
