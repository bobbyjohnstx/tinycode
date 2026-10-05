package main

import (
	"bufio"
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

// collectRunPrompt handles directory changes from positional args and collects
// the prompt text from remaining args and/or stdin. When multiTurn is true,
// stdin is not consumed (it will be read line-by-line in the multi-turn loop)
// and an empty prompt is returned instead of exiting.
func collectRunPrompt(positional []string, multiTurn bool) string {
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
	if !multiTurn && !term.IsTerminal(int(os.Stdin.Fd())) {
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
	if prompt == "" && !multiTurn {
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

// startJSONPermissionStdinRouter forwards permission_reply NDJSON lines from stdin
// to the JSON permission handler while a single-turn run is processing. Multi-turn
// mode routes permission_reply via readNextPrompt between prompts instead.
func startJSONPermissionStdinRouter(permReplyCh chan<- permission.ReplyInput) {
	safego.Go(func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var msg struct {
				Type  string `json:"type"`
				ID    string `json:"id"`
				Reply string `json:"reply"`
			}
			if err := json.Unmarshal([]byte(scanner.Text()), &msg); err != nil {
				continue
			}
			if msg.Type != "permission_reply" {
				continue
			}
			permReplyCh <- permission.ReplyInput{
				RequestID: msg.ID,
				Reply:     permission.Reply(msg.Reply),
			}
		}
	})
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
func resolveRunSession(db *storage.DB, sessionIDFlag string, cont bool, title, dir, agentName, modelID, providerID string) (string, []session.Message, *session.MessageStore) {
	store := session.NewStore(db.DB)
	ms := session.NewMessageStore(store)
	projectID := project.IDFromDirectory(dir)

	var sessionID string
	var existingMsgs []session.Message

	if sessionIDFlag != "" {
		sessionID = sessionIDFlag
		_, err := store.Get(sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: session %q not found: %v\n", sessionID, err)
			os.Exit(1)
		}
		existingMsgs, _ = ms.List(sessionID)
	} else if cont {
		sessions, err := store.List(projectID, 1, 0)
		if err != nil || len(sessions) == 0 {
			fmt.Fprintf(os.Stderr, "error: no sessions to continue\n")
			os.Exit(1)
		}
		sessionID = sessions[0].ID
		existingMsgs, _ = ms.List(sessionID)
	} else {
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

// streamRunOutput subscribes to session events and streams text/tool output
// to stdout. Goroutines run until the bus subscriptions are closed.
func streamRunOutput(b *bus.Bus, isJSON bool) {
	deltaSub := b.Subscribe("session.text.delta")
	toolBeginSub := b.Subscribe("session.tool.begin")
	toolEndSub := b.Subscribe("session.tool.end")
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
			if isJSON {
				line, _ := json.Marshal(map[string]any{
					"type":       "tool_begin",
					"toolName":   props["toolName"],
					"toolCallID": props["toolCallID"],
				})
				fmt.Println(string(line))
			}
		}
	})
	safego.Go(func() {
		for evt := range toolEndSub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			if isJSON {
				line, _ := json.Marshal(map[string]any{
					"type":       "tool_end",
					"toolName":   props["toolName"],
					"toolCallID": props["toolCallID"],
					"toolArgs":   props["toolArgs"],
				})
				fmt.Println(string(line))
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

// readNextPrompt reads one prompt from the scanner. In JSON mode it expects
// {"type":"prompt","text":"..."} and returns false on {"type":"exit"} or EOF.
// When permReplyCh is non-nil, JSON messages with type "permission_reply" are
// parsed and sent to the channel for the permission handler goroutine.
// In text mode it returns the trimmed line and false on EOF.
func readNextPrompt(scanner *bufio.Scanner, isJSON bool, permReplyCh chan<- permission.ReplyInput) (string, bool) {
	if !scanner.Scan() {
		return "", false
	}
	line := scanner.Text()

	if !isJSON {
		return strings.TrimSpace(line), true
	}

	var msg struct {
		Type      string `json:"type"`
		Text      string `json:"text"`
		ID        string `json:"id"`
		Reply     string `json:"reply"`
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
			_ = ms.Append(&newMsgs[i])
		}
		store := session.NewStore(db.DB)
		_ = store.UpdateCost(sessionID, 0, session.TokenUsage{
			Input:  result.Usage.Input,
			Output: result.Usage.Output,
		})
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
		fmt.Fprintf(os.Stderr, "error: %v\n", result.Error)
		os.Exit(1)
	}
}
