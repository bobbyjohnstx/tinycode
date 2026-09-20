package server

import (
	"log/slog"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/llm"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/tool"
)

// ToolSnapshot returns a point-in-time snapshot of the tool registry that is
// safe from concurrent MCP registration mutations.
func (sm *SessionManager) ToolSnapshot() *tool.Registry {
	return sm.toolSnapshot
}

// SetClientFactory overrides the default LLM client factory.
// This allows tests to inject mock clients without changing the constructor.
func (sm *SessionManager) SetClientFactory(f func(*provider.Model) llm.Client) {
	sm.clientFactory = f
}

func (sm *SessionManager) subscribePermissionReplies() {
	if sm.perms == nil {
		return
	}
	sub := sm.bus.Subscribe("permission.replied")
	go func() {
		for evt := range sub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			reqID, _ := props["requestID"].(string)
			replyStr, _ := props["reply"].(string)
			if reqID == "" || replyStr == "" {
				continue
			}

			var reply permission.Reply
			switch replyStr {
			case "once":
				reply = permission.ReplyOnce
			case "always":
				reply = permission.ReplyAlways
			case "reject":
				reply = permission.ReplyReject
			default:
				continue
			}

			sm.perms.RespondToAsk(permission.ReplyInput{
				RequestID: reqID,
				Reply:     reply,
			})
		}
	}()
}

// subscribeRevert listens for session.revert events and stashes the current
// working tree changes.
func (sm *SessionManager) subscribeRevert() {
	sub := sm.bus.Subscribe("session.revert")
	go func() {
		for evt := range sub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			sessionID, _ := props["sessionID"].(string)
			if sessionID == "" {
				continue
			}

			dir := sm.sessionDir(sessionID)
			if err := sm.revertState.Stash(dir, sessionID); err != nil {
				slog.Error("revert failed", "sessionID", sessionID, "error", err)
				sm.bus.Publish("session.error", map[string]any{
					"sessionID": sessionID,
					"error":     sessionErrorPayload("UnknownError", "revert failed: "+err.Error()),
				})
				continue
			}
			sm.bus.Publish("session.reverted", map[string]any{"sessionID": sessionID})
		}
	}()
}

// subscribeUnrevert listens for session.unrevert events and pops the stash
// created by the corresponding revert.
func (sm *SessionManager) subscribeUnrevert() {
	sub := sm.bus.Subscribe("session.unrevert")
	go func() {
		for evt := range sub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			sessionID, _ := props["sessionID"].(string)
			if sessionID == "" {
				continue
			}

			dir := sm.sessionDir(sessionID)
			if err := sm.revertState.Pop(dir, sessionID); err != nil {
				slog.Error("unrevert failed", "sessionID", sessionID, "error", err)
				sm.bus.Publish("session.error", map[string]any{
					"sessionID": sessionID,
					"error":     sessionErrorPayload("UnknownError", "unrevert failed: "+err.Error()),
				})
				continue
			}
			sm.bus.Publish("session.unreverted", map[string]any{"sessionID": sessionID})
		}
	}()
}

// subscribeSummarize listens for session.summarize events and publishes
// status + compacted events. Full LLM-driven compaction is handled by the
// Processor; this subscriber signals that a manual summarize was requested.
func (sm *SessionManager) subscribeSummarize() {
	sub := sm.bus.Subscribe("session.summarize")
	go func() {
		for evt := range sub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			sessionID, _ := props["sessionID"].(string)
			if sessionID == "" {
				continue
			}

			sm.bus.Publish("session.status", map[string]any{
				"sessionID": sessionID,
				"status":    map[string]any{"type": "busy"},
			})

			sm.bus.Publish("session.compacted", map[string]any{
				"sessionID": sessionID,
				"message":   "Manual summarize requested",
			})

			sm.bus.Publish("session.status", map[string]any{
				"sessionID": sessionID,
				"status":    map[string]any{"type": "idle"},
			})
		}
	}()
}

// extractAllowedPerms returns the set of permission names that are explicitly
// allowed in the ruleset. If no explicit allows are found, returns nil (meaning
// all tools should be included).
func extractAllowedPerms(ruleset permission.Ruleset) []string {
	var result []string
	for _, rule := range ruleset {
		if rule.Action == permission.ActionAllow {
			result = append(result, rule.Permission)
		}
	}
	return result
}

func (sm *SessionManager) bridgeWarning(evt bus.Event) {
	props, ok := evt.Properties.(map[string]any)
	if !ok {
		return
	}
	sessionID, _ := props["sessionID"].(string)
	message, _ := props["message"].(string)

	sm.bus.Publish("toast", map[string]any{
		"sessionID": sessionID,
		"type":      "warning",
		"message":   message,
	})
}
