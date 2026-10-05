package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/project"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/storage"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
)

const maxNDJSONToolOutput = 16 * 1024

// collectRunPrompt handles directory changes from positional args and collects
// the prompt text from remaining args and/or stdin. When multiTurn is true,
// stdin is not consumed (it will be read line-by-line in the multi-turn loop)
// and an empty prompt is returned instead of exiting. When permsJSON is true,
// stdin is reserved for the stdinMux so io.ReadAll is skipped; the prompt must
// come from positional args or the mux's prompt channel.
func collectRunPrompt(positional []string, multiTurn, permsJSON bool) string {
	if len(positional) > 0 {
		if info, err := os.Stat(positional[0]); err == nil && info.IsDir() {
			absDir, err := filepath.Abs(positional[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "resolving directory: %v\n", err)
				os.Exit(1)
			}
			if err := os.Chdir(absDir); err != nil {
				fmt.Fprintf(os.Stderr, "changing directory: %v\n", err)
				os.Exit(1)
			}
			positional = positional[1:]
		}
	}

	prompt := strings.Join(positional, " ")
	if !multiTurn && !permsJSON && !term.IsTerminal(int(os.Stdin.Fd())) {
		stdinData, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "reading stdin: %v\n", err)
			os.Exit(1)
		}
		if s := strings.TrimSpace(string(stdinData)); s != "" {
			if prompt != "" {
				prompt += "\n\n"
			}
			prompt += s
		}
	}
	if prompt == "" && !multiTurn && !permsJSON {
		fmt.Fprintf(os.Stderr, "error: no prompt provided\n")
		os.Exit(1)
	}
	return prompt
}

// setupRunPermissions configures the permission behavior for headless mode.
// When permsMode is "json", permission requests are emitted as NDJSON on stdout
// and replies are expected via the returned channel. Returns a channel for
// routing permission replies (nil when not in JSON mode).
func setupRunPermissions(b *bus.Bus, permSvc *permission.Service, skipPerms, interactive bool, permsMode string) chan<- permission.ReplyInput {
	permSub := b.Subscribe("permission.asked")
	if permsMode == "json" {
		permReplyCh := make(chan permission.ReplyInput, 16)
		safego.Go(func() { handleJSONPermissionEvents(permSub, permSvc, permReplyCh) })
		return permReplyCh
	}
	safego.Go(func() { handlePermissionEvents(permSub, permSvc, skipPerms, interactive) })
	return nil
}

// stdinMux multiplexes stdin when --permissions json is active. A single
// goroutine owns stdin for the process lifetime, dispatching JSON messages
// to the appropriate channel based on their type field.
type stdinMux struct {
	prompts     chan string
	permReplyCh chan<- permission.ReplyInput
	isJSON      bool
}

func newStdinMux(isJSON bool, permReplyCh chan<- permission.ReplyInput) *stdinMux {
	return &stdinMux{
		// Buffer generously so a pending prompt cannot block the mux from
		// delivering a permission_reply while Process is blocked on Ask.
		prompts:     make(chan string, 64),
		permReplyCh: permReplyCh,
		isJSON:      isJSON,
	}
}

func (m *stdinMux) run() {
	defer close(m.prompts)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		if m.dispatch(scanner.Text()) {
			return
		}
	}
}

// dispatch handles one stdin line. Permission replies are recognized whenever
// the mux is running (--permissions json), including when stdout format is
// plain text. It returns true when the mux should stop.
func (m *stdinMux) dispatch(line string) bool {
	if id, reply, ok := parsePermissionReply(line); ok {
		if m.permReplyCh != nil {
			select {
			case m.permReplyCh <- permission.ReplyInput{
				RequestID: id,
				Reply:     permission.Reply(reply),
			}:
			default:
				slog.Warn("permission reply channel full, dropping reply", "id", id)
			}
		}
		return false
	}
	if !m.isJSON {
		trimmed := strings.TrimSpace(line)
		if trimmed == "exit" || trimmed == "quit" {
			return true
		}
		m.sendPrompt(trimmed)
		return false
	}

	var msg struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		m.sendPrompt(strings.TrimSpace(line))
		return false
	}
	switch msg.Type {
	case "prompt":
		m.sendPrompt(msg.Text)
	case "exit":
		return true
	}
	return false
}

func (m *stdinMux) sendPrompt(text string) {
	select {
	case m.prompts <- text:
	default:
		slog.Warn("prompt channel full, dropping prompt")
	}
}

func parsePermissionReply(line string) (id, reply string, ok bool) {
	var msg struct {
		Type  string `json:"type"`
		ID    string `json:"id"`
		Reply string `json:"reply"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return "", "", false
	}
	if msg.Type != "permission_reply" {
		return "", "", false
	}
	return msg.ID, msg.Reply, true
}

// handleJSONPermissionEvents emits permission requests as NDJSON on stdout and
// reads replies from the permReplyCh channel.
func handleJSONPermissionEvents(permSub *bus.Subscription, permSvc *permission.Service, permReplyCh <-chan permission.ReplyInput) {
	safego.Go(func() {
		for reply := range permReplyCh {
			for attempt := 0; attempt < 200; attempt++ {
				err := permSvc.RespondToAsk(reply)
				if err == nil {
					break
				}
				if !errors.Is(err, permission.ErrNotFound) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
		}
	})
	for evt := range permSub.C {
		req, ok := evt.Properties.(permission.Request)
		if !ok {
			continue
		}
		line, _ := json.Marshal(map[string]any{
			"type":       "permission",
			"id":         req.ID,
			"permission": req.Permission,
			"patterns":   req.Patterns,
		})
		fmt.Println(string(line))
	}
}

// handlePermissionEvents processes permission requests from the event bus.
// In non-interactive or skip-permissions mode, it auto-replies. In interactive
// mode, it prompts the user on stderr.
func handlePermissionEvents(permSub *bus.Subscription, permSvc *permission.Service, skipPerms, interactive bool) {
	var scanner *bufio.Scanner
	if interactive && !skipPerms {
		scanner = bufio.NewScanner(os.Stdin)
	}
	autoReply := permission.ReplyReject
	if skipPerms {
		autoReply = permission.ReplyOnce
	}

	for evt := range permSub.C {
		req, ok := evt.Properties.(permission.Request)
		if !ok {
			continue
		}
		reply := autoReply
		if scanner != nil {
			fmt.Fprintf(os.Stderr, "Permission requested: %s %v\nAllow? [y/N]: ", req.Permission, req.Patterns)
			if scanner.Scan() && strings.TrimSpace(strings.ToLower(scanner.Text())) == "y" {
				reply = permission.ReplyOnce
			}
		}
		permSvc.RespondToAsk(permission.ReplyInput{
			RequestID: req.ID,
			Reply:     reply,
		})
	}
	if scanner != nil {
		if err := scanner.Err(); err != nil {
			slog.Warn("stdin scanner error", "error", err)
		}
	}
}

// resolveRunModel resolves the model from flags or config and returns the
// provider ID, model ID, and model object. Exits on error.
func resolveRunModel(reg *provider.Registry, modelStr string) (string, string, *provider.Model) {
	if modelStr == "" {
		fmt.Fprintf(os.Stderr, "error: no model specified (use --model or set model in config)\n")
		os.Exit(1)
	}

	providerID, modelID := provider.ParseModel(modelStr)
	if providerID == "" {
		models := reg.ListModels()
		for _, m := range models {
			if m.ID == modelID || m.Name == modelID {
				providerID = m.ProviderID
				modelID = m.ID
				break
			}
		}
	}
	if providerID == "" {
		fmt.Fprintf(os.Stderr, "error: could not find model %q — specify as provider/model\n", modelStr)
		os.Exit(1)
	}

	model, err := reg.GetModel(providerID, modelID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return providerID, modelID, model
}

// buildRunAgentPrompt resolves the agent and builds the system prompt.
// appendSystemPrompt is appended to the end of the system prompt if non-empty.
func buildRunAgentPrompt(agentFlag string, cfg *config.Info, agentReg *agent.Registry, model *provider.Model, dir string, toolReg *tool.Registry, appendSystemPrompt ...string) (string, []string, string) {
	agentName := agentFlag
	if agentName == "" {
		agentName = cfg.DefaultAgent
	}
	if agentName == "" {
		agentName = "build"
	}

	agentInfo := agentReg.Get(agentName, model.SizeB())
	var agentPrompt string
	var agentPerms []string
	if agentInfo != nil {
		agentPrompt = agentInfo.Prompt
		for _, rule := range agentInfo.Permission {
			if rule.Action == permission.ActionAllow {
				agentPerms = append(agentPerms, rule.Permission)
			}
		}
		// Exclude tools denied by the agent's permission rules so the LLM
		// never attempts to call them (deny rules are evaluated last-wins).
		disabled := permission.Disabled(toolReg.List(), agentInfo.Permission)
		toolReg.SetDisabled(disabled)
	}

	var instructions string
	if len(cfg.Instructions) > 0 {
		instructions = strings.Join(cfg.Instructions, "\n\n")
	}
	var appendSP string
	if len(appendSystemPrompt) > 0 {
		appendSP = appendSystemPrompt[0]
	}
	systemPrompt := session.BuildSystemPrompt(session.SystemPromptInput{
		AgentPrompt:        agentPrompt,
		Instructions:       instructions,
		Directory:          dir,
		ToolDefs:           toolReg.ToolDefs(agentPerms),
		AppendSystemPrompt: appendSP,
	})

	return agentName, agentPerms, systemPrompt
}

// resolveRunSession creates or continues a session. Returns session ID,
// existing messages, and the message store. Exits on error.
// resumeFlag resolves like TUI --resume (ID, then title/slug). sessionIDFlag
// requires an exact session ID (-s/--session).
func resolveRunSession(db *storage.DB, sessionIDFlag, resumeFlag string, cont bool, title, dir, agentName, modelID, providerID string) (string, []session.Message, *session.MessageStore) {
	store := session.NewStore(db.DB)
	ms := session.NewMessageStore(store)
	projectID := project.IDFromDirectory(dir)

	var sessionID string
	var existingMsgs []session.Message

	switch {
	case resumeFlag != "":
		sessionID = resolveResumeSessionID(store, projectID, resumeFlag)
		if sessionID == "" {
			fmt.Fprintf(os.Stderr, "error: session %q not found\n", resumeFlag)
			os.Exit(1)
		}
		existingMsgs, _ = ms.List(sessionID)
	case sessionIDFlag != "":
		sessionID = sessionIDFlag
		_, err := store.Get(sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: session %q not found: %v\n", sessionID, err)
			os.Exit(1)
		}
		existingMsgs, _ = ms.List(sessionID)
	case cont:
		sessions, err := store.List(projectID, 1, 0)
		if err != nil || len(sessions) == 0 {
			fmt.Fprintf(os.Stderr, "error: no sessions to continue\n")
			os.Exit(1)
		}
		sessionID = sessions[0].ID
		if title != "" {
			if err := store.UpdateTitle(sessionID, title); err != nil {
				slog.Warn("failed to update session title", "error", err)
			}
		}
		existingMsgs, _ = ms.List(sessionID)
	default:
		info, err := store.Create(session.CreateInput{
			ProjectID: projectID,
			Directory: dir,
			Title:     title,
			Agent:     agentName,
			Model:     &session.ModelRef{ModelID: modelID, ProviderID: providerID},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating session: %v\n", err)
			os.Exit(1)
		}
		sessionID = info.ID
	}

	return sessionID, existingMsgs, ms
}

// resolveResumeSessionID tries session ID first, then title/slug match.
func resolveResumeSessionID(store *session.Store, projectID, resume string) string {
	if info, err := store.Get(resume); err == nil {
		return info.ID
	}
	sessions, err := store.List(projectID, 50, 0)
	if err != nil {
		return ""
	}
	for _, s := range sessions {
		if s.Title == resume || s.Slug == resume {
			return s.ID
		}
	}
	return ""
}

// streamRunOutput subscribes to session events and streams text/tool output
// to stdout. Goroutines run until the bus subscriptions are closed.
func streamRunOutput(b *bus.Bus, isJSON bool) {
	deltaSub := b.Subscribe("session.text.delta")
	toolBeginSub := b.Subscribe("session.tool.begin")
	toolCallEndSub := b.Subscribe("session.tool.end")
	toolResultSub := b.Subscribe("session.tool.result")
	msgSub := b.Subscribe("session.message")
	stepStartSub := b.Subscribe("session.step.start")
	stepFinishSub := b.Subscribe("session.step.finish")
	warnSub := b.Subscribe("session.warning")
	compactSub := b.Subscribe("session.compacted")

	safego.Go(func() {
		for evt := range deltaSub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			text, _ := props["text"].(string)
			if isJSON {
				line, _ := json.Marshal(map[string]any{"type": "text", "text": text})
				fmt.Println(string(line))
			} else {
				fmt.Print(text)
			}
		}
	})
	safego.Go(func() {
		for evt := range toolBeginSub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			toolName, _ := props["toolName"].(string)
			toolCallID, _ := props["toolCallID"].(string)
			if isJSON {
				line, _ := json.Marshal(map[string]any{
					"type":       "tool_begin",
					"toolName":   toolName,
					"toolCallID": toolCallID,
				})
				fmt.Println(string(line))
			} else {
				fmt.Fprintf(os.Stderr, "tool %s begin\n", toolName)
			}
		}
	})
	safego.Go(func() {
		for evt := range toolCallEndSub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			if !isJSON {
				continue
			}
			// LLM finished streaming the tool call args (not execution result).
			line, _ := json.Marshal(map[string]any{
				"type":       "tool_call_end",
				"toolName":   props["toolName"],
				"toolCallID": props["toolCallID"],
				"toolArgs":   props["toolArgs"],
			})
			fmt.Println(string(line))
		}
	})
	safego.Go(func() {
		for evt := range toolResultSub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			toolName, _ := props["toolName"].(string)
			toolCallID, _ := props["toolCallID"].(string)
			output, _ := props["output"].(string)
			isError, _ := props["isError"].(bool)
			if isJSON {
				line, _ := json.Marshal(map[string]any{
					"type":       "tool_end",
					"toolName":   toolName,
					"toolCallID": toolCallID,
					"output":     truncateStr(output, maxNDJSONToolOutput),
					"isError":    isError,
				})
				fmt.Println(string(line))
			} else {
				status := "ok"
				if isError {
					status = "error"
				}
				fmt.Fprintf(os.Stderr, "tool %s end (%s)\n", toolName, status)
			}
		}
	})
	safego.Go(func() {
		for evt := range msgSub.C {
			if !isJSON {
				continue
			}
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			msg, ok := props["message"].(session.Message)
			if !ok {
				continue
			}
			for _, part := range msg.Parts {
				if part.Type == session.PartReasoning {
					line, _ := json.Marshal(map[string]any{
						"type":      "reasoning",
						"sessionID": msg.SessionID,
						"text":      part.Text,
					})
					fmt.Println(string(line))
				}
			}
		}
	})
	safego.Go(func() {
		for evt := range stepStartSub.C {
			if !isJSON {
				continue
			}
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			line, _ := json.Marshal(map[string]any{
				"type":      "step_start",
				"stepID":    props["stepID"],
				"iteration": props["iteration"],
				"model":     props["model"],
			})
			fmt.Println(string(line))
		}
	})
	safego.Go(func() {
		for evt := range stepFinishSub.C {
			if !isJSON {
				continue
			}
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			line, _ := json.Marshal(map[string]any{
				"type":      "step_finish",
				"stepID":    props["stepID"],
				"iteration": props["iteration"],
				"usage":     props["usage"],
				"error":     props["error"],
			})
			fmt.Println(string(line))
		}
	})
	safego.Go(func() {
		for evt := range warnSub.C {
			if !isJSON {
				continue
			}
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			line, _ := json.Marshal(map[string]any{
				"type":    "warning",
				"message": props["message"],
			})
			fmt.Println(string(line))
		}
	})
	safego.Go(func() {
		for evt := range compactSub.C {
			if !isJSON {
				continue
			}
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			line, _ := json.Marshal(map[string]any{
				"type":          "compacted",
				"compactionNum": props["compactionNum"],
				"preMessages":   props["preMessages"],
				"postMessages":  props["postMessages"],
			})
			fmt.Println(string(line))
		}
	})
}

// readNextPrompt reads one prompt. When mux is non-nil, prompts are read from
// the mux's channel (permission replies are already dispatched by the mux).
// Otherwise it reads directly from the scanner using the original line-by-line
// protocol. In JSON mode it expects {"type":"prompt","text":"..."} and returns
// false on {"type":"exit"} or EOF. In text mode it returns the trimmed line
// and false on EOF.
func readNextPrompt(scanner *bufio.Scanner, isJSON bool, permReplyCh chan<- permission.ReplyInput, mux *stdinMux) (string, bool) {
	if mux != nil {
		p, ok := <-mux.prompts
		return p, ok
	}

	if !scanner.Scan() {
		return "", false
	}
	line := scanner.Text()

	if !isJSON {
		trimmed := strings.TrimSpace(line)
		if trimmed == "exit" || trimmed == "quit" {
			return "", false
		}
		return trimmed, true
	}

	var msg struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		ID    string `json:"id"`
		Reply string `json:"reply"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return "", true
	}

	switch msg.Type {
	case "prompt":
		return msg.Text, true
	case "exit":
		return "", false
	case "permission_reply":
		if permReplyCh != nil {
			permReplyCh <- permission.ReplyInput{
				RequestID: msg.ID,
				Reply:     permission.Reply(msg.Reply),
			}
		}
		return "", true
	default:
		return "", true
	}
}

// persistRunResult saves processor results to the message store and updates
// session cost. Returns the updated existing messages slice for the next turn.
func persistRunResult(result *session.ProcessResult, existingMsgs []session.Message, ms *session.MessageStore, db *storage.DB, sessionID string) []session.Message {
	if result != nil && len(result.Messages) > len(existingMsgs) {
		newMsgs := result.Messages[len(existingMsgs):]
		for i := range newMsgs {
			if err := ms.Append(&newMsgs[i]); err != nil {
				slog.Warn("failed to persist message", "error", err)
			}
		}
		store := session.NewStore(db.DB)
		if err := store.UpdateCost(sessionID, 0, session.TokenUsage{
			Input:  result.Usage.Input,
			Output: result.Usage.Output,
		}); err != nil {
			slog.Warn("failed to update session cost", "error", err)
		}
		return result.Messages
	}
	return existingMsgs
}

// finishRun persists processor results and handles errors.
func finishRun(result *session.ProcessResult, existingMsgs []session.Message, ms *session.MessageStore, db *storage.DB, sessionID string, isJSON bool) {
	persistRunResult(result, existingMsgs, ms, db, sessionID)

	if !isJSON {
		fmt.Println() // ensure trailing newline
	}

	if result != nil && result.Error != nil {
		emitRunError(sessionID, result.Error, isJSON)
		os.Exit(exitCodeForRunError(result.Error))
	}
	emitRunDone(sessionID, isJSON)
}

func emitRunSession(sessionID string, isJSON bool) {
	if isJSON {
		line, _ := json.Marshal(map[string]any{"type": "session", "sessionID": sessionID})
		fmt.Println(string(line))
		return
	}
	fmt.Fprintf(os.Stderr, "session: %s\n", sessionID)
}

func emitRunDone(sessionID string, isJSON bool) {
	if !isJSON {
		return
	}
	line, _ := json.Marshal(map[string]any{"type": "done", "sessionID": sessionID, "ok": true})
	fmt.Println(string(line))
}

func emitRunError(sessionID string, err error, isJSON bool) {
	if isJSON {
		line, _ := json.Marshal(map[string]any{
			"type":      "error",
			"sessionID": sessionID,
			"message":   err.Error(),
		})
		fmt.Println(string(line))
		return
	}
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
}

func exitCodeForRunError(err error) int {
	if errors.Is(err, context.Canceled) {
		return 130
	}
	return 1
}
