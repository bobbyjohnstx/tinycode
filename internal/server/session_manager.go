package server

import (
	"context"
	"database/sql"
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

	mu            sync.Mutex
	assistMsgID   string            // bridge-generated assistant message ID during streaming
	textPartID    string            // bridge-generated text part ID during streaming
	streamStarted bool              // whether initial assistant events have been emitted
	msgStartTime  int64             // timestamp when the current assistant message started
	idMap         map[string]string // processor msg ID → bridge msg ID
	deltaBatcher  *deltaBatcher     // 16ms debounce for text deltas
	toolPartIDs   map[string]string // toolCallID → partID mapping for begin/end pairing
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

// SessionStatus represents the processing state of a session.
// The SPA expects a discriminated union: {"type": "idle"} or {"type": "busy"}.
type SessionStatus struct {
	Type string `json:"type"`
}

type SessionManager struct {
	mu                 sync.Mutex
	sessions           map[string]*activeSession
	bus                *bus.Bus
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
	tokenBudget        int
	sessionStartHook   func(sessionID string) []string
	modelWarmup        func(context.Context, *provider.Model)
	subagentStreams    map[string]*activeSession // streaming state for subagent synthetic IDs
	ctx                context.Context
	ctxCancel          context.CancelFunc
}

func NewSessionManager(b *bus.Bus, reg *provider.Registry, db *sql.DB, dir string, tools *tool.Registry, perms *permission.Service, agents *agent.Registry, mcpSvc *mcp.Service, cfg *config.Info, jm *session.JobManager) *SessionManager {
	ctx, cancel := context.WithCancel(context.Background())
	sm := &SessionManager{
		sessions:        make(map[string]*activeSession),
		subagentStreams: make(map[string]*activeSession),
		bus:             b,
		registry:        reg,
		db:              db,
		dir:             dir,
		tools:           tools,
		perms:           perms,
		agentRegistry:   agents,
		mcpSvc:          mcpSvc,
		cfg:             cfg,
		revertState:     NewRevertState(),
		jobManager:      jm,
		ctx:             ctx,
		ctxCancel:       cancel,
	}
	sm.clientFactory = func(m *provider.Model) llm.Client {
		sm.warmupModel(m)
		apiKey, _ := m.Options["api_key"].(string)
		if apiKey == "" {
			if info, err := reg.GetProvider(m.ProviderID); err == nil {
				if k, ok := info.Options["apiKey"].(string); ok {
					apiKey = k
				}
			}
		}
		if apiKey == "" && sm.credentialLookup != nil {
			apiKey = sm.credentialLookup(m.ProviderID)
		}
		if strings.Contains(m.API.URL, "api.anthropic.com") {
			return llm.NewAnthropicClient(m.API.URL, apiKey)
		}
		return llm.NewOpenAIClient(m.API.URL+"/v1", apiKey)
	}
	sm.subscribeCommands()
	sm.subscribePrompts()
	sm.subscribePermissionReplies()
	sm.subscribeProcessorEvents()
	sm.subscribeRevert()
	sm.subscribeUnrevert()
	sm.subscribeSummarize()

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

// subscribeProcessorEvents subscribes to Processor bus events and re-publishes
// them as UI events that the TUI and web clients expect.
func (sm *SessionManager) subscribeProcessorEvents() {
	msgSub := sm.bus.Subscribe("session.message")
	deltaSub := sm.bus.Subscribe("session.text.delta")
	toolBeginSub := sm.bus.Subscribe("session.tool.begin")
	toolEndSub := sm.bus.Subscribe("session.tool.end")
	warnSub := sm.bus.Subscribe("session.warning")

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
// background jobs via the JobManager.
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
