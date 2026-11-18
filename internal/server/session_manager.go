package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/llm"
	"github.com/bobbyjohnstx/tinycode/internal/mcp"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
)

type activeSession struct {
	cancel    context.CancelFunc
	processor *session.Processor
	model     *provider.Model
	agent     string
	dir       string        // session's working directory (may differ from server default)
	done      chan struct{} // closed when processPrompt returns

	mu               sync.Mutex
	assistMsgID      string            // bridge-generated assistant message ID during streaming
	textPartID       string            // bridge-generated text part ID during streaming
	reasoningPartID  string            // bridge-generated reasoning part ID during streaming
	streamStarted    bool              // whether initial assistant events have been emitted
	msgStartTime     int64             // timestamp when the current assistant message started
	idMap            map[string]string // processor msg ID → bridge msg ID
	deltaBatcher     *deltaBatcher     // 16ms debounce for text deltas
	reasoningBatcher *deltaBatcher     // 16ms debounce for reasoning deltas
	toolPartIDs      map[string]string // toolCallID → partID mapping for begin/end pairing

	// Durable across prompts (Processor is recreated each turn).
	compactionFailures     int
	recentToolCalls        []session.ToolCallSignature
	goalActive             bool
	compactBreakerTripped  bool
	thinkingBudgetDisabled bool
}

// idleCompactState tracks a pending or running idle compaction for a session.
type idleCompactState struct {
	timer  *time.Timer
	cancel context.CancelFunc // cancels the running Compact goroutine (nil until timer fires)
}

const deltaBatchInterval = 16 * time.Millisecond

type deltaBatcher struct {
	mu    sync.Mutex
	buf   strings.Builder
	timer *time.Timer
	flush func(text string)
}

func newDeltaBatcher(flush func(string)) *deltaBatcher {
	return &deltaBatcher{flush: flush}
}

func (b *deltaBatcher) Add(text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.WriteString(text)
	if b.timer == nil {
		b.timer = time.AfterFunc(deltaBatchInterval, b.doFlush)
	} else {
		b.timer.Reset(deltaBatchInterval)
	}
}

func (b *deltaBatcher) Flush() {
	b.mu.Lock()
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	text := b.buf.String()
	b.buf.Reset()
	b.mu.Unlock()
	if text != "" {
		b.flush(text)
	}
}

func (b *deltaBatcher) doFlush() {
	b.mu.Lock()
	b.timer = nil
	text := b.buf.String()
	b.buf.Reset()
	b.mu.Unlock()
	if text != "" {
		b.flush(text)
	}
}

// ShellHookSetter allows setting agent-scoped hooks on a shell hook runner
// without importing the plugin package.
type ShellHookSetter interface {
	SetAgentHooks(hooks map[string][]config.HookConfig, agentName string)
}

// SessionStatus represents the processing state of a session.
// The SPA expects a discriminated union: {"type": "idle"} or {"type": "busy"}.
type SessionStatus struct {
	Type string `json:"type"`
}

type SessionManager struct {
	mu                 sync.Mutex
	sessions           map[string]*activeSession
	bus                *bus.Bus
	busSubs            []*bus.Subscription
	registry           *provider.Registry
	db                 *sql.DB
	dir                string
	tools              *tool.Registry
	toolSnapshot       *tool.Registry
	perms              *permission.Service
	agentRegistry      *agent.Registry
	mcpSvc             *mcp.Service
	cfg                *config.Info
	revertState        *RevertState
	clientFactory      func(*provider.Model) llm.Client
	credentialLookup   func(providerID string) string
	jobManager         *session.JobManager
	appendSystemPrompt string
	notesDir           string
	tokenBudget        int
	sessionStartHook   func(sessionID string) []string
	modelWarmup        func(context.Context, *provider.Model)
	shellHookRunner    ShellHookSetter
	subagentStreams        map[string]*activeSession // streaming state for subagent synthetic IDs
	pendingJobNotify      map[string][]string       // sessionID → pending background-task notifications
	abortedSessions       map[string]bool           // sessions the user interrupted (cleared on next prompt)
	idleCompactTimers     map[string]*idleCompactState // sessionID → pending idle compaction
	ctx                   context.Context
	ctxCancel             context.CancelFunc
}

func NewSessionManager(b *bus.Bus, reg *provider.Registry, db *sql.DB, dir string, tools *tool.Registry, perms *permission.Service, agents *agent.Registry, mcpSvc *mcp.Service, cfg *config.Info, jm *session.JobManager) *SessionManager {
	ctx, cancel := context.WithCancel(context.Background())
	sm := &SessionManager{
		sessions:          make(map[string]*activeSession),
		subagentStreams:    make(map[string]*activeSession),
		pendingJobNotify:  make(map[string][]string),
		abortedSessions:   make(map[string]bool),
		idleCompactTimers: make(map[string]*idleCompactState),
		bus:              b,
		registry:         reg,
		db:               db,
		dir:              dir,
		tools:            tools,
		perms:            perms,
		agentRegistry:    agents,
		mcpSvc:           mcpSvc,
		cfg:              cfg,
		revertState:      NewRevertState(),
		jobManager:       jm,
		ctx:              ctx,
		ctxCancel:        cancel,
	}
	sm.clientFactory = func(m *provider.Model) llm.Client {
		sm.warmupModel(m)
		// Precedence: env → config/options → auth.json (credentialLookup).
		var envVars []string
		opts := map[string]any{}
		if info, err := reg.GetProvider(m.ProviderID); err == nil {
			envVars = info.Env
			for k, v := range info.Options {
				opts[k] = v
			}
		}
		for k, v := range m.Options {
			opts[k] = v
		}
		apiKey := ""
		for _, name := range envVars {
			if name == "" {
				continue
			}
			if v := os.Getenv(name); v != "" {
				apiKey = v
				break
			}
		}
		if apiKey == "" {
			if k, ok := opts["apiKey"].(string); ok {
				apiKey = k
			}
		}
		if apiKey == "" {
			if k, ok := opts["api_key"].(string); ok {
				apiKey = k
			}
		}
		if apiKey == "" && sm.credentialLookup != nil {
			apiKey = sm.credentialLookup(m.ProviderID)
		}
		return llm.NewClient(m.API.URL, apiKey, m.API.NPM, m.ProviderID)
	}
	sm.subscribeCommands()
	sm.subscribePrompts()
	sm.subscribePermissionReplies()
	sm.subscribeProcessorEvents()
	sm.subscribeRevert()
	sm.subscribeUnrevert()
	sm.subscribeSummarize()
	sm.subscribeJobCompleted()

	// Take a baseline snapshot of the tool registry before any MCP tools are
	// registered. Session processing should use ToolSnapshot() to get a
	// point-in-time copy that is safe from concurrent MCP mutations.
	if sm.tools != nil {
		sm.toolSnapshot = sm.tools.Snapshot()
	}

	return sm
}

// SetModelWarmup registers a hook invoked when a prompt is about to call a
// model. Used to probe the selected Ollama model instead of every discovered
// model at startup.
func (sm *SessionManager) SetModelWarmup(fn func(context.Context, *provider.Model)) {
	sm.mu.Lock()
	sm.modelWarmup = fn
	sm.mu.Unlock()
}

// SetShellHookRunner stores a reference to the shell hook runner for
// setting agent-scoped hooks on agent switches.
func (sm *SessionManager) SetShellHookRunner(r ShellHookSetter) {
	sm.mu.Lock()
	sm.shellHookRunner = r
	sm.mu.Unlock()
}

func (sm *SessionManager) warmupModel(m *provider.Model) {
	if m == nil {
		return
	}
	sm.mu.Lock()
	fn := sm.modelWarmup
	sm.mu.Unlock()
	if fn == nil {
		return
	}
	fn(sm.ctx, m)
}

// idleCompactDelay is how long the session must be idle before background
// compaction starts. Exported as a variable so tests can shorten it.
var idleCompactDelay = 5 * time.Minute

// cancelIdleCompaction stops any pending or running idle compaction for the session.
// Must be called with sm.mu NOT held.
func (sm *SessionManager) cancelIdleCompaction(sessionID string) {
	sm.mu.Lock()
	state, ok := sm.idleCompactTimers[sessionID]
	if ok {
		delete(sm.idleCompactTimers, sessionID)
	}
	sm.mu.Unlock()

	if !ok {
		return
	}
	state.timer.Stop()
	if state.cancel != nil {
		state.cancel()
	}
}

// startIdleCompactionTimer schedules background compaction for a session that
// just went idle, if estimated input tokens exceed 80% of the context window.
func (sm *SessionManager) startIdleCompactionTimer(sessionID string, lastInputTokens int, model *provider.Model) {
	if !config.IdleCompactionEnabled(sm.cfg) {
		return
	}
	if model == nil || model.Limit.Context <= 0 {
		return
	}

	threshold := int(float64(model.Limit.Context) * 0.8)
	if lastInputTokens < threshold {
		return
	}

	slog.Info("scheduling idle compaction", "sessionID", sessionID,
		"inputTokens", lastInputTokens, "threshold", threshold,
		"delay", idleCompactDelay)

	timer := time.AfterFunc(idleCompactDelay, func() {
		sm.runIdleCompaction(sessionID)
	})

	sm.mu.Lock()
	sm.idleCompactTimers[sessionID] = &idleCompactState{timer: timer}
	sm.mu.Unlock()
}

// runIdleCompaction executes background compaction when the idle timer fires.
func (sm *SessionManager) runIdleCompaction(sessionID string) {
	ctx, cancel := context.WithCancel(sm.ctx)

	sm.mu.Lock()
	state, ok := sm.idleCompactTimers[sessionID]
	if !ok {
		// Timer was cancelled between firing and this goroutine starting.
		sm.mu.Unlock()
		cancel()
		return
	}
	state.cancel = cancel
	// If the session became busy in the meantime, bail out.
	if _, busy := sm.sessions[sessionID]; busy {
		delete(sm.idleCompactTimers, sessionID)
		sm.mu.Unlock()
		cancel()
		return
	}
	sm.mu.Unlock()

	slog.Info("starting idle compaction", "sessionID", sessionID)

	compacted, err := sm.Summarize(ctx, sessionID)

	sm.mu.Lock()
	delete(sm.idleCompactTimers, sessionID)
	sm.mu.Unlock()
	cancel()

	if err != nil {
		slog.Warn("idle compaction failed", "sessionID", sessionID, "error", err)
		return
	}
	if !compacted {
		slog.Info("idle compaction: nothing to compact", "sessionID", sessionID)
		return
	}

	slog.Info("idle compaction completed", "sessionID", sessionID)
	sm.bus.Publish("toast", map[string]any{
		"sessionID": sessionID,
		"type":      "info",
		"message":   "Session compacted while idle",
	})
}

func (sm *SessionManager) trackSub(sub *bus.Subscription) *bus.Subscription {
	sm.busSubs = append(sm.busSubs, sub)
	return sub
}

// subscribeProcessorEvents subscribes to Processor bus events and re-publishes
// them as UI events that the TUI and web clients expect.
func (sm *SessionManager) subscribeProcessorEvents() {
	msgSub := sm.trackSub(sm.bus.Subscribe("session.message"))
	deltaSub := sm.trackSub(sm.bus.Subscribe("session.text.delta"))
	reasoningSub := sm.trackSub(sm.bus.Subscribe("session.reasoning.delta"))
	toolBeginSub := sm.trackSub(sm.bus.Subscribe("session.tool.begin"))
	toolEndSub := sm.trackSub(sm.bus.Subscribe("session.tool.end"))
	warnSub := sm.trackSub(sm.bus.Subscribe("session.warning"))

	safego.Go(func() {
		for evt := range msgSub.C {
			sm.bridgeMessageEvent(evt)
		}
	})
	safego.Go(func() {
		for evt := range deltaSub.C {
			sm.bridgeTextDelta(evt)
		}
	})
	safego.Go(func() {
		for evt := range reasoningSub.C {
			sm.bridgeReasoningDelta(evt)
		}
	})
	safego.Go(func() {
		for evt := range toolBeginSub.C {
			sm.bridgeToolBegin(evt)
		}
	})
	safego.Go(func() {
		for evt := range toolEndSub.C {
			sm.bridgeToolEnd(evt)
		}
	})
	safego.Go(func() {
		for evt := range warnSub.C {
			sm.bridgeWarning(evt)
		}
	})
}

// Shutdown cancels all active session processors and waits for them
// to finish persisting before returning. It also cancels all running
// background jobs via the JobManager and unsubscribes bus bridge loops.
func (sm *SessionManager) Shutdown() {
	sm.ctxCancel()
	sm.mu.Lock()
	var doneChans []chan struct{}
	for sid, active := range sm.sessions {
		if active.cancel != nil {
			active.cancel()
		}
		if active.done != nil {
			doneChans = append(doneChans, active.done)
		}
		delete(sm.sessions, sid)
	}
	// Stop all idle compaction timers and running goroutines.
	for sid, state := range sm.idleCompactTimers {
		state.timer.Stop()
		if state.cancel != nil {
			state.cancel()
		}
		delete(sm.idleCompactTimers, sid)
	}
	sm.mu.Unlock()

	if sm.jobManager != nil {
		sm.jobManager.Shutdown()
	}

	if sm.tools != nil {
		sm.tools.ShutdownMonitors()
	}

	for _, done := range doneChans {
		<-done
	}

	for _, sub := range sm.busSubs {
		sub.Unsubscribe()
	}
	sm.busSubs = nil

	tool.ClearFileMutexes()
}

// Status returns the processing status of all active sessions.
func (sm *SessionManager) Status() map[string]SessionStatus {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	result := make(map[string]SessionStatus, len(sm.sessions))
	for sid := range sm.sessions {
		result[sid] = SessionStatus{Type: "busy"}
	}
	return result
}

func (sm *SessionManager) getActive(sessionID string) *activeSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.sessions[sessionID]
}

// sessionDir returns the working directory for a session, falling back to sm.dir.
func (sm *SessionManager) sessionDir(sessionID string) string {
	store := session.NewStore(sm.db)
	if info, err := store.Get(sessionID); err == nil && info.Directory != "" {
		return info.Directory
	}
	return sm.dir
}

// subscribeJobCompleted listens for background job completions and either
// wakes the parent session immediately (if idle and not user-interrupted)
// or queues the notification for delivery at the next turn boundary.
func (sm *SessionManager) subscribeJobCompleted() {
	sub := sm.trackSub(sm.bus.Subscribe("job.completed"))
	safego.Go(func() {
		for evt := range sub.C {
			props, ok := evt.Properties.(map[string]any)
			if !ok {
				continue
			}
			parentID, _ := props["parentSessionID"].(string)
			if parentID == "" {
				continue
			}
			jobID, _ := props["jobID"].(string)
			status, _ := props["status"].(string)
			result, _ := props["result"].(string)
			jobErr, _ := props["error"].(string)

			notification := formatJobNotification(jobID, status, result, jobErr)

			sm.mu.Lock()
			_, busy := sm.sessions[parentID]
			aborted := sm.abortedSessions[parentID]
			if busy || aborted {
				// Parent is processing or was interrupted — queue for later.
				sm.pendingJobNotify[parentID] = append(sm.pendingJobNotify[parentID], notification)
				sm.mu.Unlock()
				slog.Info("queued background task notification", "jobID", jobID, "parentSession", parentID, "reason", map[bool]string{true: "busy", false: "aborted"}[busy])
				continue
			}
			sm.mu.Unlock()

			// Parent is idle and not interrupted — wake it.
			slog.Info("waking parent session for background task", "jobID", jobID, "parentSession", parentID)
			sm.bus.Publish("session.prompt", map[string]any{
				"sessionID": parentID,
				"content":   notification,
			})
		}
	})
}

// drainJobNotifications returns any queued background-task notifications for
// the given session and clears the queue. Returns empty string if none.
func (sm *SessionManager) drainJobNotifications(sessionID string) string {
	sm.mu.Lock()
	pending := sm.pendingJobNotify[sessionID]
	delete(sm.pendingJobNotify, sessionID)
	sm.mu.Unlock()

	if len(pending) == 0 {
		return ""
	}
	return strings.Join(pending, "\n\n---\n\n")
}

// formatJobNotification builds a human-readable notification string for a
// completed background task.
func formatJobNotification(jobID, status, result, jobErr string) string {
	switch status {
	case "completed":
		return fmt.Sprintf("Background task %s completed (success):\n\n%s", jobID, result)
	case "failed":
		return fmt.Sprintf("Background task %s failed:\n\n%s", jobID, jobErr)
	case "cancelled":
		return fmt.Sprintf("Background task %s was cancelled.", jobID)
	default:
		return fmt.Sprintf("Background task %s finished (status: %s).", jobID, status)
	}
}
