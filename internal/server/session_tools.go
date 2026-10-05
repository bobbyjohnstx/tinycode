package server

import (
	"log/slog"

	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
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
	safego.Go(func() {
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
	})
}

// subscribeRevert listens for session.revert events and stashes the current
// working tree changes.
func (sm *SessionManager) subscribeRevert() {
	sub := sm.bus.Subscribe("session.revert")
	safego.Go(func() {
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
	})
}

// subscribeUnrevert listens for session.unrevert events and pops the stash
// created by the corresponding revert.
func (sm *SessionManager) subscribeUnrevert() {
	sub := sm.bus.Subscribe("session.unrevert")
	safego.Go(func() {
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
	})
}

// subscribeSummarize listens for session.summarize events and publishes
// status + compacted events. Full LLM-driven compaction is handled by the
// Processor; this subscriber signals that a manual summarize was requested.
func (sm *SessionManager) subscribeSummarize() {
	sub := sm.bus.Subscribe("session.summarize")
	safego.Go(func() {
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
				"sessionID":     sessionID,
				"compactionNum": 0,
				"message":       "Manual summarize requested",
			})

			sm.bus.Publish("session.status", map[string]any{
				"sessionID": sessionID,
				"status":    map[string]any{"type": "idle"},
			})
		}
	})
}

// extractAllowedPerms returns the set of permission names that are effectively
// allowed after applying last-wins semantics to the merged ruleset.
// A later deny rule overrides an earlier allow for the same permission.
func extractAllowedPerms(ruleset permission.Ruleset) []string {
	// Collect all unique non-wildcard permission names from the ruleset.
	seen := make(map[string]struct{})
	for _, rule := range ruleset {
		if rule.Permission != "*" {
			seen[rule.Permission] = struct{}{}
		}
	}

	var result []string
	for perm := range seen {
		// Last-wins: scan backward, first matching rule determines action.
		for i := len(ruleset) - 1; i >= 0; i-- {
			if permission.WildcardMatch(perm, ruleset[i].Permission) {
				if ruleset[i].Action == permission.ActionAllow {
					result = append(result, perm)
				}
				break
			}
		}
	}

	// Check if the wildcard permission itself is effectively allowed.
	for i := len(ruleset) - 1; i >= 0; i-- {
		if ruleset[i].Permission == "*" {
			if ruleset[i].Action == permission.ActionAllow {
				result = append(result, "*")
			}
			break
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
