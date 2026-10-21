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
	"sync"
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
	"github.com/bobbyjohnstx/tinycode/internal/server"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
)

const maxNDJSONToolOutput = 16 * 1024

// stdoutMu serializes NDJSON lines so concurrent emitters never interleave bytes.
var stdoutMu sync.Mutex

func writeNDJSON(v map[string]any) {
	line, err := json.Marshal(v)
	if err != nil {
		return
	}
	stdoutMu.Lock()
	fmt.Println(string(line))
	stdoutMu.Unlock()
}

func truncateNDJSONOutput(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	return s[:max], true
}

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
	setupRunQuestionReject(b)
	permSub := b.Subscribe("permission.asked")
	if permsMode == "json" {
		permReplyCh := make(chan permission.ReplyInput, 16)
		safego.Go(func() { handleJSONPermissionEvents(permSub, permSvc, permReplyCh) })
		return permReplyCh
	}
	safego.Go(func() { handlePermissionEvents(permSub, permSvc, skipPerms, interactive) })
	return nil
}

// wireRunSubagentRunner attaches Server.RunSubagent to the shared tool context
// so the task tool works in headless run (same path as ACP/TUI, without Listen).
func wireRunSubagentRunner(toolCtx *tool.Context, srv *server.Server) {
	if toolCtx == nil || srv == nil {
		return
	}
	toolCtx.SubagentRunner = func(subCtx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
		return srv.RunSubagent(subCtx, parentSessionID, parentDepth, prompt, agent, directory, autoApprove)
	}
}

// setupRunQuestionReject fail-closes the question tool in headless run mode.
// Mirrors ACP: publish question.rejected so executeQuestion returns IsError
// immediately instead of hanging on stdin.
func setupRunQuestionReject(b *bus.Bus) {
	asked := b.Subscribe("question.asked")
	safego.Go(func() {
		for evt := range asked.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			questionID, _ := props["id"].(string)
			sessionID, _ := props["sessionID"].(string)
			if questionID == "" || sessionID == "" {
				continue
			}
			// Defer reject so executeQuestion can subscribe to question.rejected
			// after publishing asked (same race ACP avoids via safego.Go).
			id, sid := questionID, sessionID
			safego.Go(func() {
				b.Publish("question.rejected", map[string]any{
					"sessionID": sid,
					"requestID": id,
					"reason":    "question tool is unsupported in non-interactive run mode",
				})
			})
		}
	})
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
			normalized, valid := normalizePermissionReply(reply)
			if !valid {
				fmt.Fprintf(os.Stderr, "error: invalid permission reply %q for %s (want once, always, or reject); rejecting\n", reply, id)
				normalized = permission.ReplyReject
			}
			deliverPermissionReply(m.permReplyCh, permission.ReplyInput{
				RequestID: id,
				Reply:     normalized,
			})
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

func validatePermissionsFlag(mode string) error {
	switch mode {
	case "default", "json", "":
		return nil
	default:
		return fmt.Errorf("unknown --permissions %q (want default or json)", mode)
	}
}

// validateRunStdinFlags rejects unsafe stdin ownership combinations before Process.
func validateRunStdinFlags(interactive, multiTurn, permsJSON, formatJSON, stdinIsTTY bool) error {
	if interactive && multiTurn {
		return fmt.Errorf("-i/--interactive cannot be combined with --multi-turn")
	}
	if interactive && permsJSON {
		return fmt.Errorf("-i/--interactive cannot be combined with --permissions json")
	}
	if interactive && !stdinIsTTY {
		return fmt.Errorf("-i/--interactive requires a TTY stdin (not a pipe)")
	}
	if permsJSON && !formatJSON {
		return fmt.Errorf("--permissions json requires --format json")
	}
	return nil
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

func normalizePermissionReply(reply string) (permission.Reply, bool) {
	switch permission.Reply(reply) {
	case permission.ReplyOnce, permission.ReplyAlways, permission.ReplyReject:
		return permission.Reply(reply), true
	default:
		return permission.ReplyReject, false
	}
}

// deliverPermissionReply sends a reply without silently dropping. Blocks up to
// 5s for channel space; on timeout force-sends a reject if possible.
func deliverPermissionReply(ch chan<- permission.ReplyInput, reply permission.ReplyInput) {
	select {
	case ch <- reply:
		return
	case <-time.After(5 * time.Second):
		slog.Error("permission reply channel blocked; forcing reject", "id", reply.RequestID)
		fmt.Fprintf(os.Stderr, "error: permission reply channel blocked for %s; rejecting\n", reply.RequestID)
		reject := permission.ReplyInput{RequestID: reply.RequestID, Reply: permission.ReplyReject}
		select {
		case ch <- reject:
		case <-time.After(time.Second):
			slog.Error("permission reply channel still blocked after reject attempt", "id", reply.RequestID)
		}
	}
}

// handleJSONPermissionEvents emits permission requests as NDJSON on stdout and
// reads replies from the permReplyCh channel.
func handleJSONPermissionEvents(permSub *bus.Subscription, permSvc *permission.Service, permReplyCh <-chan permission.ReplyInput) {
	safego.Go(func() {
		for reply := range permReplyCh {
			if _, valid := normalizePermissionReply(string(reply.Reply)); !valid {
				fmt.Fprintf(os.Stderr, "error: invalid permission reply %q for %s; rejecting\n", reply.Reply, reply.RequestID)
				reply.Reply = permission.ReplyReject
			}
			if applyPermissionReply(permSvc, reply) {
				continue
			}
			force := permission.ReplyInput{
				RequestID: reply.RequestID,
				Reply:     permission.ReplyReject,
				Message:   "permission reply timed out waiting for pending ask",
			}
			fmt.Fprintf(os.Stderr, "error: permission reply for %s exhausted retries; rejecting\n", reply.RequestID)
			if applyPermissionReply(permSvc, force) {
				continue
			}
			// Ask may register after the reply window — keep force-rejecting briefly
			// so a late Ask is not left pending forever.
			reqID := reply.RequestID
			safego.Go(func() {
				force := permission.ReplyInput{
					RequestID: reqID,
					Reply:     permission.ReplyReject,
					Message:   "permission reply timed out waiting for pending ask",
				}
				_ = applyPermissionReply(permSvc, force)
			})
		}
	})
	for evt := range permSub.C {
		req, ok := evt.Properties.(permission.Request)
		if !ok {
			continue
		}
		payload := map[string]any{
			"type":       "permission",
			"id":         req.ID,
			"permission": req.Permission,
			"patterns":   req.Patterns,
		}
		if len(req.Metadata) > 0 {
			payload["metadata"] = req.Metadata
		}
		writeNDJSON(payload)
	}
}

// applyPermissionReply retries RespondToAsk for up to 200×10ms on ErrNotFound.
// Returns true when the reply was applied.
func applyPermissionReply(permSvc *permission.Service, reply permission.ReplyInput) bool {
	for attempt := 0; attempt < 200; attempt++ {
		err := permSvc.RespondToAsk(reply)
		if err == nil {
			return true
		}
		if !errors.Is(err, permission.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "error: permission reply for %s: %v\n", reply.RequestID, err)
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
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
func resolveRunModel(reg *provider.Registry, modelStr string, isJSON bool) (string, string, *provider.Model) {
	fail := func(msg string) {
		if isJSON {
			writeNDJSON(map[string]any{"type": "error", "message": msg})
		} else {
			fmt.Fprintf(os.Stderr, "error: %s\n", msg)
		}
		os.Exit(1)
	}
	if modelStr == "" {
		fail("no model specified (use --model, set model in config, or run tinycode doctor)")
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
		fail(fmt.Sprintf("could not find model %q — specify as provider/model (or run tinycode doctor)", modelStr))
	}

	model, err := reg.GetModel(providerID, modelID)
	if err != nil {
		fail(err.Error())
	}
	return providerID, modelID, model
}

// buildRunAgentPrompt resolves the agent and builds the system prompt.
// appendSystemPrompt is appended to the end of the system prompt if non-empty.
// The caller should apply agentRuleset via toolReg.WithAgentRules before Execute.
func buildRunAgentPrompt(agentFlag string, cfg *config.Info, agentReg *agent.Registry, model *provider.Model, dir string, toolReg *tool.Registry, appendSystemPrompt ...string) (agentName string, agentPerms []string, agentRuleset permission.Ruleset, systemPrompt string) {
	agentName = agentFlag
	if agentName == "" {
		agentName = cfg.DefaultAgent
	}
	if agentName == "" {
		agentName = "build"
	}

	agentInfo := agentReg.Get(agentName, model.SizeB())
	var agentPrompt string
	tools := toolReg
	if agentInfo != nil {
		agentPrompt = agentInfo.Prompt
		agentRuleset = agentInfo.Permission
		agentPerms = permission.Visible(agentInfo.Permission)
		tools = toolReg.WithAgentRules(agentInfo.Permission)
	}

	var instructions string
	if len(cfg.Instructions) > 0 {
		instructions = strings.Join(cfg.Instructions, "\n\n")
	}
	var appendSP string
	if len(appendSystemPrompt) > 0 {
		appendSP = appendSystemPrompt[0]
	}
	systemPrompt = session.BuildSystemPrompt(session.SystemPromptInput{
		AgentPrompt:        agentPrompt,
		Instructions:       instructions,
		Directory:          dir,
		ToolDefs:           tools.ToolDefs(agentPerms),
		AppendSystemPrompt: appendSP,
	})

	return agentName, agentPerms, agentRuleset, systemPrompt
}

// resolveRunSession creates or continues a session. Returns session ID,
// existing messages, and the message store.
// resumeFlag resolves like TUI --resume (ID, then title/slug). sessionIDFlag
// requires an exact session ID (-s/--session).
func resolveRunSession(db *storage.DB, sessionIDFlag, resumeFlag string, cont bool, title, dir, agentName, modelID, providerID string) (string, []session.Message, *session.MessageStore, error) {
	store := session.NewStore(db.DB)
	ms := session.NewMessageStore(store)
	projectID := project.IDFromDirectory(dir)

	var sessionID string
	var existingMsgs []session.Message

	var err error
	switch {
	case resumeFlag != "":
		sessionID = resolveResumeSessionID(store, projectID, resumeFlag)
		if sessionID == "" {
			return "", nil, nil, fmt.Errorf("session %q not found", resumeFlag)
		}
		existingMsgs, err = ms.List(sessionID)
		if err != nil {
			return "", nil, nil, fmt.Errorf("list messages for session %q: %w", sessionID, err)
		}
	case sessionIDFlag != "":
		sessionID = sessionIDFlag
		if _, err = store.Get(sessionID); err != nil {
			return "", nil, nil, fmt.Errorf("session %q not found: %w", sessionID, err)
		}
		existingMsgs, err = ms.List(sessionID)
		if err != nil {
			return "", nil, nil, fmt.Errorf("list messages for session %q: %w", sessionID, err)
		}
	case cont:
		sessions, err := store.List(projectID, 1, 0, false)
		if err != nil {
			return "", nil, nil, fmt.Errorf("list sessions: %w", err)
		}
		if len(sessions) == 0 {
			return "", nil, nil, fmt.Errorf("no sessions to continue")
		}
		sessionID = sessions[0].ID
		if title != "" {
			if err := store.UpdateTitle(sessionID, title); err != nil {
				slog.Warn("failed to update session title", "error", err)
			}
		}
		existingMsgs, err = ms.List(sessionID)
		if err != nil {
			return "", nil, nil, fmt.Errorf("list messages for session %q: %w", sessionID, err)
		}
	default:
		info, err := store.Create(session.CreateInput{
			ProjectID: projectID,
			Directory: dir,
			Title:     title,
			Agent:     agentName,
			Model:     &session.ModelRef{ModelID: modelID, ProviderID: providerID},
		})
		if err != nil {
			return "", nil, nil, fmt.Errorf("creating session: %w", err)
		}
		sessionID = info.ID
	}

	return sessionID, existingMsgs, ms, nil
}

// resolveResumeSessionID tries session ID first, then title/slug match.
func resolveResumeSessionID(store *session.Store, projectID, resume string) string {
	if info, err := store.Get(resume); err == nil {
		return info.ID
	}
	sessions, err := store.List(projectID, 50, 0, false)
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

// runToolErrorTracker records whether any tool_end event had isError:true.
type runToolErrorTracker struct {
	mu  sync.Mutex
	saw bool
}

func (t *runToolErrorTracker) mark() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.saw = true
	t.mu.Unlock()
}

func (t *runToolErrorTracker) seen() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.saw
}

// streamRunOutput subscribes to session events and streams text/tool output
// to stdout. Goroutines run until the bus subscriptions are closed.
// When toolErrs is non-nil, tool_end events with isError:true call mark().
func streamRunOutput(b *bus.Bus, isJSON bool, toolErrs *runToolErrorTracker) {
	deltaSub := b.Subscribe("session.text.delta")
	toolBeginSub := b.Subscribe("session.tool.begin")
	toolCallEndSub := b.Subscribe("session.tool.end")
	toolResultSub := b.Subscribe("session.tool.result")
	msgSub := b.Subscribe("session.message")
	reasoningSub := b.Subscribe("session.reasoning.delta")
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
				writeNDJSON(map[string]any{"type": "text", "text": text})
			} else {
				stdoutMu.Lock()
				fmt.Print(text)
				stdoutMu.Unlock()
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
				writeNDJSON(map[string]any{
					"type":       "tool_begin",
					"toolName":   toolName,
					"toolCallID": toolCallID,
				})
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
			writeNDJSON(map[string]any{
				"type":       "tool_call_end",
				"toolName":   props["toolName"],
				"toolCallID": props["toolCallID"],
				"toolArgs":   props["toolArgs"],
			})
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
			if isError {
				toolErrs.mark()
			}
			if isJSON {
				out, truncated := truncateNDJSONOutput(output, maxNDJSONToolOutput)
				payload := map[string]any{
					"type":       "tool_end",
					"toolName":   toolName,
					"toolCallID": toolCallID,
					"output":     out,
					"isError":    isError,
				}
				if truncated {
					payload["truncated"] = true
				}
				writeNDJSON(payload)
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
		for evt := range reasoningSub.C {
			if !isJSON {
				continue
			}
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			writeNDJSON(map[string]any{
				"type":      "reasoning",
				"sessionID": props["sessionID"],
				"text":      props["text"],
			})
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
					writeNDJSON(map[string]any{
						"type":      "reasoning",
						"sessionID": msg.SessionID,
						"text":      part.Text,
					})
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
			writeNDJSON(map[string]any{
				"type":      "step_start",
				"stepID":    props["stepID"],
				"iteration": props["iteration"],
				"model":     props["model"],
			})
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
			writeNDJSON(map[string]any{
				"type":      "step_finish",
				"stepID":    props["stepID"],
				"iteration": props["iteration"],
				"usage":     props["usage"],
				"error":     props["error"],
			})
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
			writeNDJSON(map[string]any{
				"type":    "warning",
				"message": props["message"],
			})
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
			writeNDJSON(map[string]any{
				"type":          "compacted",
				"compactionNum": props["compactionNum"],
				"preMessages":   props["preMessages"],
				"postMessages":  props["postMessages"],
			})
		}
	})
}

// readNextPrompt reads one prompt. When mux is non-nil, prompts are read from
// the mux's channel (permission replies are already dispatched by the mux).
// Otherwise it reads directly from the scanner using the original line-by-line
// protocol. In JSON mode it expects {"type":"prompt","text":"..."} and returns
// false on {"type":"exit"} or EOF. In text mode it returns the trimmed line
// and false on EOF. When ctx is canceled (SIGINT), returns ("", false) promptly
// so the caller can exit 130 without waiting for stdin EOF.
func readNextPrompt(ctx context.Context, scanner *bufio.Scanner, isJSON bool, permReplyCh chan<- permission.ReplyInput, mux *stdinMux) (string, bool) {
	if mux != nil {
		return readMuxPrompt(ctx, mux)
	}

	type scanResult struct {
		line string
		ok   bool
	}
	ch := make(chan scanResult, 1)
	safego.Go(func() {
		if scanner.Scan() {
			ch <- scanResult{scanner.Text(), true}
			return
		}
		ch <- scanResult{"", false}
	})

	var line string
	select {
	case <-ctx.Done():
		return "", false
	case r := <-ch:
		if !r.ok {
			return "", false
		}
		line = r.line
	}

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
			normalized, valid := normalizePermissionReply(msg.Reply)
			if !valid {
				fmt.Fprintf(os.Stderr, "error: invalid permission reply %q for %s (want once, always, or reject); rejecting\n", msg.Reply, msg.ID)
				normalized = permission.ReplyReject
			}
			deliverPermissionReply(permReplyCh, permission.ReplyInput{
				RequestID: msg.ID,
				Reply:     normalized,
			})
		}
		return "", true
	default:
		return "", true
	}
}

func readMuxPrompt(ctx context.Context, mux *stdinMux) (string, bool) {
	select {
	case <-ctx.Done():
		return "", false
	case p, ok := <-mux.prompts:
		return p, ok
	}
}

// exitOnRunCancel emits a best-effort error event and exits 130 when ctx was canceled.
func exitOnRunCancel(ctx context.Context, sessionID string, isJSON bool) {
	if ctx.Err() == nil {
		return
	}
	emitRunError(sessionID, ctx.Err(), isJSON)
	os.Exit(130)
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
func finishRun(result *session.ProcessResult, existingMsgs []session.Message, ms *session.MessageStore, db *storage.DB, sessionID string, isJSON bool, failOnToolError bool, toolErrs *runToolErrorTracker) {
	persistRunResult(result, existingMsgs, ms, db, sessionID)

	if !isJSON {
		fmt.Println() // ensure trailing newline
	}

	if result != nil && result.Error != nil {
		emitRunError(sessionID, result.Error, isJSON)
		os.Exit(exitCodeForRunError(result.Error))
	}
	ok := !(failOnToolError && toolErrs.seen())
	emitRunDone(sessionID, isJSON, ok)
	if !ok {
		os.Exit(1)
	}
}

func emitRunSession(sessionID string, isJSON bool) {
	if isJSON {
		writeNDJSON(map[string]any{"type": "session", "sessionID": sessionID})
		return
	}
	fmt.Fprintf(os.Stderr, "session: %s\n", sessionID)
}

func emitRunDone(sessionID string, isJSON bool, ok bool) {
	if !isJSON {
		return
	}
	writeNDJSON(map[string]any{"type": "done", "sessionID": sessionID, "ok": ok})
}

func emitRunError(sessionID string, err error, isJSON bool) {
	if isJSON {
		payload := map[string]any{
			"type":    "error",
			"message": err.Error(),
		}
		if sessionID != "" {
			payload["sessionID"] = sessionID
		}
		writeNDJSON(payload)
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
