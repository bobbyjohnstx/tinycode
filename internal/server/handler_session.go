package server

import (
	"context"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	id2 "github.com/bobbyjohnstx/tinycode/internal/id"
	"github.com/bobbyjohnstx/tinycode/internal/project"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
)

func (s *Server) sessionStore() *session.Store {
	return session.NewStore(s.deps.DB)
}

func (s *Server) messageStore() *session.MessageStore {
	return session.NewMessageStore(s.sessionStore())
}

func (s *Server) partStore() *session.PartStore {
	return session.NewPartStore(s.deps.DB)
}

func (s *Server) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ParentID string            `json:"parentID,omitempty"`
		Title    string            `json:"title,omitempty"`
		Agent    string            `json:"agent,omitempty"`
		Model    *session.ModelRef `json:"model,omitempty"`
	}

	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Assign a default agent if none was provided.
	if body.Agent == "" {
		if s.config.DefaultAgent != "" {
			body.Agent = s.config.DefaultAgent
		} else {
			body.Agent = "build"
		}
	}

	// Assign a default model if none was provided.
	if body.Model == nil {
		body.Model = s.resolveDefaultModel()
	}

	dir := requestDirectory(r, s.config.Directory)

	projectID := project.IDFromDirectory(dir)

	store := s.sessionStore()
	info, err := store.Create(session.CreateInput{
		ProjectID: projectID,
		Directory: dir,
		ParentID:  body.ParentID,
		Title:     body.Title,
		Agent:     body.Agent,
		Model:     body.Model,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.deps.Bus.Publish("project.updated", project.FromDirectory(dir))

	s.deps.Bus.Publish("session.created", map[string]any{
		"sessionID": info.ID,
		"info":      info,
	})

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	info, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionList(w http.ResponseWriter, r *http.Request) {
	dir := requestDirectory(r, s.config.Directory)

	projectID := project.IDFromDirectory(dir)

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	store := s.sessionStore()
	sessions, err := store.List(projectID, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if sessions == nil {
		sessions = []session.Info{}
	}

	respondJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleSessionUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Title string `json:"title,omitempty"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	store := s.sessionStore()

	if body.Title != "" {
		if err := store.UpdateTitle(id, body.Title); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	info, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	s.deps.Bus.Publish("session.updated", map[string]any{
		"sessionID": id,
		"info":      info,
	})

	respondJSON(w, http.StatusOK, info)
}

func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	// Fetch session info before deletion so we can include it in the event.
	info, _ := store.Get(id)

	if err := store.Delete(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	evt := map[string]any{
		"sessionID": id,
	}
	if info != nil {
		evt["info"] = info
	}
	s.deps.Bus.Publish("session.deleted", evt)

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSessionArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	store := s.sessionStore()

	if err := store.Archive(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	info, _ := store.Get(id)
	if info != nil {
		s.deps.Bus.Publish("session.updated", map[string]any{
			"sessionID": id,
			"info":      info,
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSessionPrompt(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	var body struct {
		Content        string       `json:"content,omitempty"`
		MessageID      string       `json:"messageID,omitempty"`
		Model          *promptModel `json:"model,omitempty"`
		Agent          string       `json:"agent,omitempty"`
		Parts          []promptPart `json:"parts"`
		Variant        string       `json:"variant,omitempty"`
		ThinkingBudget *int         `json:"thinkingBudget,omitempty"`
		MaxTokens      *int         `json:"maxTokens,omitempty"`
		SystemPrefix   string       `json:"systemPrefix,omitempty"`
		MaxIterations  *int         `json:"maxIterations,omitempty"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Support legacy {"content": "..."} format
	if len(body.Parts) == 0 && body.Content != "" {
		body.Parts = []promptPart{{Type: "text", Text: body.Content}}
	}

	if len(body.Parts) == 0 {
		respondError(w, http.StatusBadRequest, "parts or content is required")
		return
	}

	// Resolve model and agent from session store when not provided
	if body.Model == nil || body.Agent == "" {
		store := s.sessionStore()
		info, err := store.Get(sessionID)
		if err == nil && info != nil {
			if body.Model == nil && info.Model != nil {
				body.Model = &promptModel{
					ProviderID: info.Model.ProviderID,
					ModelID:    info.Model.ModelID,
				}
			}
			if body.Agent == "" {
				body.Agent = info.Agent
			}
		}
	}

	s.sessionManager.StartPrompt(context.WithoutCancel(r.Context()), PromptInput{
		SessionID:      sessionID,
		Model:          body.Model,
		Agent:          body.Agent,
		Parts:          body.Parts,
		MessageID:      body.MessageID,
		ThinkingBudget: body.ThinkingBudget,
		MaxTokens:      body.MaxTokens,
		SystemPrefix:   body.SystemPrefix,
		MaxIterations:  body.MaxIterations,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSessionPromptAsync(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")

	var body struct {
		MessageID      string       `json:"messageID,omitempty"`
		Model          *promptModel `json:"model,omitempty"`
		Agent          string       `json:"agent,omitempty"`
		Parts          []promptPart `json:"parts"`
		Variant        string       `json:"variant,omitempty"`
		ThinkingBudget *int         `json:"thinkingBudget,omitempty"`
		MaxTokens      *int         `json:"maxTokens,omitempty"`
		SystemPrefix   string       `json:"systemPrefix,omitempty"`
		MaxIterations  *int         `json:"maxIterations,omitempty"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Resolve model and agent from session store when not provided in the request body.
	if body.Model == nil || body.Agent == "" {
		store := s.sessionStore()
		info, err := store.Get(sessionID)
		if err == nil && info != nil {
			if body.Model == nil && info.Model != nil {
				body.Model = &promptModel{
					ProviderID: info.Model.ProviderID,
					ModelID:    info.Model.ModelID,
				}
			}
			if body.Agent == "" {
				body.Agent = info.Agent
			}
		}
	}

	// Detach from request cancellation — this handler returns 204 immediately
	// while the prompt processes asynchronously. WithoutCancel preserves
	// request-scoped values without tying the prompt's lifetime to the HTTP request.
	s.sessionManager.StartPrompt(context.WithoutCancel(r.Context()), PromptInput{
		SessionID:      sessionID,
		Model:          body.Model,
		Agent:          body.Agent,
		Parts:          body.Parts,
		MessageID:      body.MessageID,
		ThinkingBudget: body.ThinkingBudget,
		MaxTokens:      body.MaxTokens,
		SystemPrefix:   body.SystemPrefix,
		MaxIterations:  body.MaxIterations,
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSessionShell(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")

	var body struct {
		MessageID string       `json:"messageID,omitempty"`
		Model     *promptModel `json:"model,omitempty"`
		Agent     string       `json:"agent,omitempty"`
		Command   string       `json:"command"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	command := strings.TrimSpace(body.Command)
	if command == "" {
		respondError(w, http.StatusBadRequest, "command is required")
		return
	}

	dir := s.config.Directory
	store := s.sessionStore()
	if info, err := store.Get(sessionID); err == nil && info != nil && info.Directory != "" {
		dir = info.Directory
	}

	safego.Go(func() { s.executeShellDirect(sessionID, command, dir) })
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) executeShellDirect(sessionID, command, dir string) {
	b := s.deps.Bus
	now := time.Now().UnixMilli()

	b.Publish("session.status", map[string]any{
		"sessionID": sessionID,
		"status":    map[string]any{"type": "busy"},
	})
	defer func() {
		b.Publish("session.status", map[string]any{
			"sessionID": sessionID,
			"status":    map[string]any{"type": "idle"},
		})
	}()

	userMsgID, _ := id2.Ascending("message")
	b.Publish("message.updated", map[string]any{
		"sessionID": sessionID,
		"info": map[string]any{
			"id":        userMsgID,
			"sessionID": sessionID,
			"role":      "user",
			"time":      map[string]any{"created": now},
		},
	})
	userPartID, _ := id2.Ascending("part")
	b.Publish("message.part.updated", map[string]any{
		"sessionID": sessionID,
		"part": map[string]any{
			"id":        userPartID,
			"sessionID": sessionID,
			"messageID": userMsgID,
			"type":      "text",
			"text":      "! " + command,
			"time":      map[string]any{"start": now, "end": now},
		},
		"time": now,
	})

	var output string
	var isErr bool

	// Block destructive commands (same patterns the bash tool checks).
	if tool.IsDestructive(command) {
		output = "Destructive command blocked: " + command + "\nUse the bash tool in an agent session for destructive operations."
		isErr = true
	} else {
		// Enforce a timeout matching the bash tool default (120s).
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		stdout := tool.NewLimitedWriter(tool.MaxOutputSize)
		stderr := tool.NewLimitedWriter(tool.MaxOutputSize)
		cmd := exec.CommandContext(ctx, "sh", "-c", command)
		cmd.Dir = dir
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		cmdErr := cmd.Run()

		var outputBuf strings.Builder
		if stdout.Len() > 0 {
			outputBuf.Write(stdout.Bytes())
			if stdout.Overflow {
				outputBuf.WriteString("\n[output truncated at 10MB]")
			}
		}
		if stderr.Len() > 0 {
			if outputBuf.Len() > 0 {
				outputBuf.WriteString("\n")
			}
			outputBuf.WriteString("STDERR:\n")
			outputBuf.Write(stderr.Bytes())
			if stderr.Overflow {
				outputBuf.WriteString("\n[stderr truncated at 10MB]")
			}
		}
		output = outputBuf.String()
		isErr = cmdErr != nil
		if isErr && output == "" {
			output = cmdErr.Error()
		}
	}

	input := map[string]any{"command": command}
	completedAt := time.Now().UnixMilli()
	toolCallID, _ := id2.Ascending("tool")

	assistMsgID, _ := id2.Ascending("message")
	b.Publish("message.updated", map[string]any{
		"sessionID": sessionID,
		"info": map[string]any{
			"id":        assistMsgID,
			"sessionID": sessionID,
			"role":      "assistant",
			"time":      map[string]any{"created": now, "completed": completedAt},
			"parentID":  userMsgID,
		},
	})

	toolPartID, _ := id2.Ascending("part")
	state := map[string]any{
		"status":   "completed",
		"input":    input,
		"output":   output,
		"title":    command,
		"metadata": map[string]any{"output": output},
		"time":     map[string]any{"start": now, "end": completedAt},
	}
	if isErr {
		state = map[string]any{
			"status":   "error",
			"input":    input,
			"error":    output,
			"metadata": map[string]any{},
			"time":     map[string]any{"start": now, "end": completedAt},
		}
	}
	b.Publish("message.part.updated", map[string]any{
		"sessionID": sessionID,
		"part": map[string]any{
			"id":        toolPartID,
			"sessionID": sessionID,
			"messageID": assistMsgID,
			"type":      "tool",
			"callID":    toolCallID,
			"tool":      "bash",
			"state":     state,
		},
		"time": completedAt,
	})
}

func (s *Server) handleSessionAbort(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	s.sessionManager.Abort(id)

	s.deps.Bus.Publish("session.abort", map[string]any{
		"sessionID": id,
	})

	respondJSON(w, http.StatusOK, map[string]any{
		"sessionID": id,
		"status":    "aborted",
	})
}

func (s *Server) handleSessionFork(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		MessageID string `json:"messageID,omitempty"`
		Title     string `json:"title,omitempty"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	store := s.sessionStore()
	parent, err := store.Get(id)
	if err != nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}

	forked, err := store.Create(session.CreateInput{
		ProjectID: parent.ProjectID,
		Directory: parent.Directory,
		ParentID:  id,
		Title:     body.Title,
		Agent:     parent.Agent,
		Model:     parent.Model,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Copy messages from parent to forked session.
	// If body.MessageID is set, copy only up to and including that message.
	ms := s.messageStore()
	parentMsgs, err := ms.List(id)
	if err == nil {
		for i := range parentMsgs {
			msg := parentMsgs[i]
			msg.SessionID = forked.ID
			newID, idErr := id2.Ascending("message")
			if idErr != nil {
				continue
			}
			msg.ID = newID
			_ = ms.Append(&msg)

			if body.MessageID != "" && parentMsgs[i].ID == body.MessageID {
				break
			}
		}
	}

	respondJSON(w, http.StatusCreated, forked)
}

// resolveDefaultModel returns a ModelRef using the configured default model or,
// failing that, the first model from the first available provider.
func (s *Server) resolveDefaultModel() *session.ModelRef {
	// 1. Check configured default model (e.g. "ollama/llama3.2").
	if s.config.DefaultModel != "" {
		providerID, modelID := provider.ParseModel(s.config.DefaultModel)
		if providerID != "" && modelID != "" {
			if _, err := s.deps.Registry.GetModel(providerID, modelID); err == nil {
				return &session.ModelRef{ProviderID: providerID, ModelID: modelID}
			}
		}
	}

	// 2. Pick the smallest chat-capable model from local providers first.
	providers := s.deps.Registry.ListProviders()
	sort.Slice(providers, func(i, j int) bool { return providers[i].ID < providers[j].ID })

	localProviders := map[string]bool{"ollama": true, "lm-studio": true, "vllm": true}
	type candidate struct {
		providerID string
		modelID    string
		sizeB      float64
		local      bool
	}
	var candidates []candidate

	for _, p := range providers {
		for id, m := range p.Models {
			if strings.Contains(strings.ToLower(id), "embed") {
				continue
			}
			size := 999.0
			if s := m.SizeB(); s != nil {
				size = *s
			}
			candidates = append(candidates, candidate{p.ID, id, size, localProviders[p.ID]})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].local != candidates[j].local {
			return candidates[i].local
		}
		if candidates[i].sizeB != candidates[j].sizeB {
			return candidates[i].sizeB < candidates[j].sizeB
		}
		if candidates[i].providerID != candidates[j].providerID {
			return candidates[i].providerID < candidates[j].providerID
		}
		return candidates[i].modelID < candidates[j].modelID
	})
	if len(candidates) > 0 {
		return &session.ModelRef{ProviderID: candidates[0].providerID, ModelID: candidates[0].modelID}
	}

	return nil
}

func (s *Server) handleMessageList(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	store := s.sessionStore()
	ms := s.messageStore()
	ps := s.partStore()

	messages, err := ms.List(sessionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Look up session once for providerID enrichment on assistant messages.
	sessInfo, _ := store.Get(sessionID)

	result := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
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
			if sessInfo != nil && sessInfo.Model != nil {
				info["providerID"] = sessInfo.Model.ProviderID
				if m.Model == "" {
					info["modelID"] = sessInfo.Model.ModelID
				}
			}
		}
		if m.Model != "" {
			info["modelID"] = m.Model
		}

		result = append(result, map[string]any{
			"info":  info,
			"parts": parts,
		})
	}

	respondJSON(w, http.StatusOK, result)
}
