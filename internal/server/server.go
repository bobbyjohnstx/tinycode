package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/agent"
	"github.com/bobbyjohnstx/tinycode/internal/bus"
	"github.com/bobbyjohnstx/tinycode/internal/config"
	"github.com/bobbyjohnstx/tinycode/internal/mcp"
	"github.com/bobbyjohnstx/tinycode/internal/permission"
	"github.com/bobbyjohnstx/tinycode/internal/plugin"
	"github.com/bobbyjohnstx/tinycode/internal/provider"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
	"github.com/bobbyjohnstx/tinycode/internal/server/middleware"
	"github.com/bobbyjohnstx/tinycode/internal/session"
	"github.com/bobbyjohnstx/tinycode/internal/static"
	"github.com/bobbyjohnstx/tinycode/internal/tool"
)

const (
	defaultPort       = 4096
	shutdownTimeout   = 25 * time.Second
	readHeaderTimeout = 10 * time.Second
)

type Config struct {
	Port               int
	Hostname           string
	WebUIDir           string
	ServeWebUI         bool
	Directory          string
	DefaultModel       string
	DefaultAgent       string
	Token              string
	AppendSystemPrompt string
	TokenBudget        int
	Version            string
}

type Listener struct {
	Hostname string
	Port     int
	URL      *url.URL
}

type Dependencies struct {
	Bus             *bus.Bus
	DB              *sql.DB
	Registry        *provider.Registry
	AgentRegistry   *agent.Registry
	PluginManager   *plugin.Manager
	BuiltinManager  *plugin.BuiltinManager
	ShellHookRunner *plugin.ShellHookRunner
	ToolRegistry    *tool.Registry
	PermService     *permission.Service
	MCPService      *mcp.Service
	Config          *config.Info
	JobManager      *session.JobManager
	Discovery       *provider.Discovery
}

type Server struct {
	config           Config
	httpServer       *http.Server
	mux              *http.ServeMux
	deps             Dependencies
	logger           *slog.Logger
	sessionManager   *SessionManager
	permissionStore  *PermissionStore
	questionStore    *QuestionStore
	credentials      *credentialStore
	pluginSubs       []*bus.Subscription
	pluginDone       chan struct{}
	shutdownDone     chan struct{}
	sessionStartMu   sync.Mutex
	sessionStartCtx  map[string][]string // additionalContext from session.created hooks
}

func New(cfg Config, deps Dependencies) *Server {
	// Port 0 means ephemeral (OS-assigned). Do not coerce to defaultPort —
	// callers that want 4096 must set it explicitly (serverConfig does).
	if cfg.Hostname == "" {
		cfg.Hostname = "127.0.0.1"
	}

	mux := http.NewServeMux()
	logger := slog.Default()

	s := &Server{
		config:          cfg,
		mux:             mux,
		deps:            deps,
		logger:          logger,
		sessionManager:  NewSessionManager(deps.Bus, deps.Registry, deps.DB, cfg.Directory, deps.ToolRegistry, deps.PermService, deps.AgentRegistry, deps.MCPService, deps.Config, deps.JobManager),
		permissionStore: NewPermissionStore(),
		questionStore:   NewQuestionStore(),
		credentials:     newCredentialStore(),
	}

	s.sessionManager.appendSystemPrompt = cfg.AppendSystemPrompt
	s.sessionManager.tokenBudget = cfg.TokenBudget
	s.sessionManager.SetCredentialLookup(func(providerID string) string {
		creds, ok := s.credentials.Get(providerID)
		if !ok {
			return ""
		}
		if k := creds["apiKey"]; k != "" {
			return k
		}
		return creds["api_key"]
	})
	if deps.Discovery != nil {
		disc := deps.Discovery
		s.sessionManager.SetModelWarmup(func(ctx context.Context, m *provider.Model) {
			disc.Warmup(ctx, m)
		})
	}
	s.wireSessionStartHook()
	s.wirePendingStores()

	s.registerRoutes()
	s.wirePluginHooks()

	corsConfig := middleware.DefaultCORSConfig()

	var handler http.Handler
	if cfg.ServeWebUI {
		authedAPI := middleware.TokenAuth(cfg.Token)(mux)
		staticFS := static.Handler(cfg.WebUIDir)
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, pattern := mux.Handler(r)
			if pattern != "" {
				authedAPI.ServeHTTP(w, r)
				return
			}
			// Set auth cookie on initial page load with ?auth_token= so the
			// SPA's API calls are authenticated even after page reloads.
			if qt := r.URL.Query().Get("auth_token"); qt != "" && cfg.Token != "" {
				if middleware.MatchesToken("Basic "+qt, cfg.Token) {
					http.SetCookie(w, &http.Cookie{
						Name:     "tinycode_auth",
						Value:    qt,
						Path:     "/",
						HttpOnly: true,
						SameSite: http.SameSiteStrictMode,
					})
				}
			}
			staticFS.ServeHTTP(w, r)
		})
	} else {
		handler = middleware.TokenAuth(cfg.Token)(mux)
	}

	handler = middleware.CORS(corsConfig)(middleware.SecurityHeaders(handler))

	s.httpServer = &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	return s
}

func (s *Server) Listen(ctx context.Context) (*Listener, error) {
	addr := fmt.Sprintf("%s:%d", s.config.Hostname, s.config.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.logger.Warn("port in use, trying random port", "port", s.config.Port, "error", err)
		ln, err = net.Listen("tcp", fmt.Sprintf("%s:0", s.config.Hostname))
		if err != nil {
			return nil, fmt.Errorf("binding to port: %w", err)
		}
	}

	tcpAddr := ln.Addr().(*net.TCPAddr)
	actualPort := tcpAddr.Port

	u := &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s:%d", s.config.Hostname, actualPort),
	}

	listener := &Listener{
		Hostname: s.config.Hostname,
		Port:     actualPort,
		URL:      u,
	}

	s.logger.Info("server listening", "url", u.String())

	safego.Go(func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server error", "error", err)
		}
	})

	s.deps.Bus.Publish("server.connected", map[string]any{
		"url": u.String(),
	})

	s.shutdownDone = make(chan struct{})
	safego.Go(func() {
		defer close(s.shutdownDone)
		<-ctx.Done()
		s.logger.Info("shutting down server")

		if s.pluginDone != nil {
			close(s.pluginDone)
		}

		s.sessionManager.Shutdown()

		s.deps.Bus.Publish("global.disposed", map[string]any{
			"timestamp": time.Now().UnixMilli(),
		})

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("shutdown error", "error", err)
		}
	})

	return listener, nil
}

// wireSessionStartHook installs a first-prompt reader that returns
// additionalContext collected when session.created fired external/builtin hooks.
func (s *Server) wireSessionStartHook() {
	s.sessionManager.sessionStartHook = func(sessionID string) []string {
		s.sessionStartMu.Lock()
		defer s.sessionStartMu.Unlock()
		if s.sessionStartCtx == nil {
			return nil
		}
		ctx := s.sessionStartCtx[sessionID]
		delete(s.sessionStartCtx, sessionID)
		return ctx
	}
}

// wirePendingStores populates in-memory permission/question stores from bus
// ask events so GET /permission and GET /question return pending requests.
func (s *Server) wirePendingStores() {
	if s.deps.Bus == nil {
		return
	}
	if s.pluginDone == nil {
		s.pluginDone = make(chan struct{})
	}

	s.wirePluginEventLoop("permission.asked", func(props map[string]any) {
		id, _ := props["id"].(string)
		if id == "" {
			return
		}
		sid, _ := props["sessionID"].(string)
		perm, _ := props["permission"].(string)
		p := PendingPermission{
			ID:         id,
			SessionID:  sid,
			Permission: perm,
		}
		if patterns, ok := props["patterns"].([]string); ok {
			p.Patterns = patterns
		} else if raw, ok := props["patterns"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					p.Patterns = append(p.Patterns, s)
				}
			}
		}
		if meta, ok := props["metadata"].(map[string]any); ok {
			p.Metadata = meta
		}
		if always, ok := props["always"].([]string); ok {
			p.Always = always
		} else if raw, ok := props["always"].([]any); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok {
					p.Always = append(p.Always, s)
				}
			}
		}
		if tool, ok := props["tool"].(string); ok {
			p.Tool = tool
		}
		s.permissionStore.Add(p)
	})

	s.wirePluginEventLoop("permission.replied", func(props map[string]any) {
		id, _ := props["requestID"].(string)
		if id != "" {
			s.permissionStore.Remove(id)
		}
	})

	s.wirePluginEventLoop("question.asked", func(props map[string]any) {
		id, _ := props["id"].(string)
		if id == "" {
			return
		}
		sid, _ := props["sessionID"].(string)
		q := PendingQuestion{ID: id, SessionID: sid}
		if questions, ok := props["questions"].([]any); ok && len(questions) > 0 {
			if first, ok := questions[0].(map[string]any); ok {
				q.Question, _ = first["question"].(string)
				if opts, ok := first["options"].([]any); ok {
					for _, o := range opts {
						switch v := o.(type) {
						case string:
							q.Options = append(q.Options, v)
						case map[string]any:
							if label, ok := v["label"].(string); ok {
								q.Options = append(q.Options, label)
							}
						}
					}
				}
			}
		} else if question, ok := props["question"].(string); ok {
			q.Question = question
		}
		s.questionStore.Add(q)
	})

	s.wirePluginEventLoop("question.replied", func(props map[string]any) {
		id, _ := props["requestID"].(string)
		if id != "" {
			s.questionStore.Remove(id)
		}
	})
}

func (s *Server) wirePluginHooks() {
	mgr := s.deps.PluginManager
	shellRunner := s.deps.ShellHookRunner
	if mgr == nil && s.deps.BuiltinManager == nil && shellRunner == nil {
		return
	}

	if s.pluginDone == nil {
		s.pluginDone = make(chan struct{})
	}

	s.wirePluginEventLoop("session.created", func(props map[string]any) {
		info, _ := props["info"].(map[string]any)
		if info == nil {
			return
		}
		sid, _ := info["id"].(string)
		if sid == "" {
			return
		}
		// Fire builtins and external plugins together on session.created.
		if s.deps.BuiltinManager != nil {
			s.deps.BuiltinManager.DispatchHook("session.start", sid)
		}
		ctx, err := plugin.DispatchSessionStart(mgr, plugin.SessionStartEvent{
			SessionID: sid,
			Directory: s.config.Directory,
		}, shellRunner)
		if err != nil {
			s.logger.Warn("session.start hook error", "sessionID", sid, "error", err)
		}
		if len(ctx) > 0 {
			s.sessionStartMu.Lock()
			if s.sessionStartCtx == nil {
				s.sessionStartCtx = make(map[string][]string)
			}
			s.sessionStartCtx[sid] = ctx
			s.sessionStartMu.Unlock()
		}
	})

	s.wirePluginEventLoop("session.deleted", func(props map[string]any) {
		sid, _ := props["sessionID"].(string)
		if sid != "" {
			plugin.DispatchSessionEnd(mgr, plugin.SessionEndEvent{SessionID: sid}, shellRunner)
			if s.deps.BuiltinManager != nil {
				s.deps.BuiltinManager.DispatchHook("session.end", sid)
			}
		}
	})

	// permission.ask is handled synchronously via permission.Service.AskInterceptor
	// (wired in cmd/tinycode) so plugins like safety-net can deny before the UI ask.
	// shell.env, tool.execute.before, and tool.execute.after dispatch is handled
	// synchronously via tool.Context.ShellEnvHook, BeforeHook, and AfterHook
	// respectively, so hooks can mutate env, abort execution, transform output,
	// and provide additionalContext.
}

// wirePluginEventLoop subscribes to a bus topic and runs the handler in a
// goroutine that exits when pluginDone is closed.
func (s *Server) wirePluginEventLoop(topic string, handler func(map[string]any)) {
	if s.pluginDone == nil {
		s.pluginDone = make(chan struct{})
	}
	sub := s.deps.Bus.Subscribe(topic)
	s.pluginSubs = append(s.pluginSubs, sub)
	safego.Go(func() {
		for {
			select {
			case <-s.pluginDone:
				return
			case evt, ok := <-sub.C:
				if !ok {
					return
				}
				props := eventPropsMap(evt.Properties)
				if props == nil {
					continue
				}
				handler(props)
			}
		}
	})
}

// eventPropsMap converts bus event properties to a map for handlers.
func eventPropsMap(props any) map[string]any {
	if props == nil {
		return nil
	}
	if m, ok := props.(map[string]any); ok {
		return m
	}
	b, err := json.Marshal(props)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}

// WaitForShutdown blocks until the server's background shutdown goroutine
// (triggered by context cancellation in Listen) has finished draining
// all active session processors and closing the HTTP server.
func (s *Server) WaitForShutdown() {
	if s.shutdownDone != nil {
		<-s.shutdownDone
	}
}

// RunSubagent delegates to SessionManager.RunSubagent for use by the task tool.
// SessionManager returns the in-process session manager used by the HTTP API.
func (s *Server) SessionManager() *SessionManager {
	return s.sessionManager
}

func (s *Server) RunSubagent(ctx context.Context, parentSessionID string, parentDepth int, prompt, agent, directory string, autoApprove bool) (string, error) {
	return s.sessionManager.RunSubagent(ctx, parentSessionID, parentDepth, prompt, agent, directory, autoApprove)
}

func (s *Server) Shutdown(ctx context.Context) error {
	for _, sub := range s.pluginSubs {
		sub.Unsubscribe()
	}
	s.pluginSubs = nil
	return s.httpServer.Shutdown(ctx)
}
