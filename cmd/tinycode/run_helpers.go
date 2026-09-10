package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
	"github.com/bobbyjohnstx/tinycode-go/internal/project"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/session"
	"github.com/bobbyjohnstx/tinycode-go/internal/storage"

	"github.com/bobbyjohnstx/tinycode-go/internal/agent"
	"github.com/bobbyjohnstx/tinycode-go/internal/config"
	"github.com/bobbyjohnstx/tinycode-go/internal/tool"
)

// collectRunPrompt handles directory changes from positional args and collects
// the prompt text from remaining args and/or stdin.
func collectRunPrompt(positional []string) string {
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
	if !term.IsTerminal(int(os.Stdin.Fd())) {
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
	if prompt == "" {
		fmt.Fprintf(os.Stderr, "error: no prompt provided\n")
		os.Exit(1)
	}
	return prompt
}

// setupRunPermissions configures the permission behavior for headless mode.
func setupRunPermissions(b *bus.Bus, permSvc *permission.Service, skipPerms, interactive bool) {
	permSub := b.Subscribe("permission.asked")
	go handlePermissionEvents(permSub, permSvc, skipPerms, interactive)
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
func buildRunAgentPrompt(agentFlag string, cfg *config.Info, agentReg *agent.Registry, model *provider.Model, dir string, toolReg *tool.Registry) (string, []string, string) {
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
	}

	var instructions string
	if len(cfg.Instructions) > 0 {
		instructions = strings.Join(cfg.Instructions, "\n\n")
	}
	systemPrompt := session.BuildSystemPrompt(session.SystemPromptInput{
		AgentPrompt:  agentPrompt,
		Instructions: instructions,
		Directory:    dir,
		ToolDefs:     toolReg.ToolDefs(agentPerms),
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
			Model:     &session.ModelRef{ID: modelID, ProviderID: providerID},
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

	go func() {
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
	}()
	go func() {
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
	}()
	go func() {
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
	}()
}

// finishRun persists processor results and handles errors.
func finishRun(result *session.ProcessResult, existingMsgs []session.Message, ms *session.MessageStore, db *storage.DB, sessionID string, isJSON bool) {
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
	}

	if !isJSON {
		fmt.Println() // ensure trailing newline
	}

	if result != nil && result.Error != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", result.Error)
		os.Exit(1)
	}
}
