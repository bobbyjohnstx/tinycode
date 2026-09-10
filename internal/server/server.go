package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/bobbyjohnstx/tinycode-go/internal/agent"
	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/config"
	"github.com/bobbyjohnstx/tinycode-go/internal/mcp"
	"github.com/bobbyjohnstx/tinycode-go/internal/permission"
	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/server/middleware"
	"github.com/bobbyjohnstx/tinycode-go/internal/static"
	"github.com/bobbyjohnstx/tinycode-go/internal/tool"
)

const (
	defaultPort      = 4096
	shutdownTimeout  = 25 * time.Second
	readHeaderTimeout = 10 * time.Second
)

type Config struct {
	Port         int
	Hostname     string
	WebUIDir     string
	ServeWebUI   bool
	Directory    string
	DefaultModel string
	DefaultAgent string
	Token        string
}

type Listener struct {
	Hostname string
	Port     int
	URL      *url.URL
}

type Dependencies struct {
	Bus           *bus.Bus
	DB            *sql.DB
	Registry      *provider.Registry
	AgentRegistry *agent.Registry
	PluginManager *plugin.Manager
	ToolRegistry  *tool.Registry
	PermService   *permission.Service
	MCPService    *mcp.Service
	Config        *config.Info
}

type Server struct {
	config          Config
	httpServer      *http.Server
	mux             *http.ServeMux
	deps            Dependencies
	logger          *slog.Logger
	sessionManager  *SessionManager
	permissionStore *PermissionStore
	questionStore   *QuestionStore
	credentials     *credentialStore
	pluginSubs      []*bus.Subscription
	pluginDone      chan struct{}
	shutdownDone    chan struct{}
}

func New(cfg Config, deps Dependencies) *Server {
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
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
		sessionManager:  NewSessionManager(deps.Bus, deps.Registry, deps.DB, cfg.Directory, deps.ToolRegistry, deps.PermService, deps.AgentRegistry, deps.MCPService, deps.Config),
		permissionStore: NewPermissionStore(),
		questionStore:   NewQuestionStore(),
		credentials:     newCredentialStore(),
	}

	s.registerRoutes()
	s.wirePluginHooks()

	if cfg.ServeWebUI {
		mux.Handle("/", static.Handler(cfg.WebUIDir))
	}

	corsConfig := middleware.DefaultCORSConfig()
	handler := middleware.CORS(corsConfig)(middleware.TokenAuth(cfg.Token)(middleware.SecurityHeaders(mux)))

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

	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server error", "error", err)
		}
	}()

	s.deps.Bus.Publish("server.connected", map[string]any{
		"url": u.String(),
	})

	s.shutdownDone = make(chan struct{})
	go func() {
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
	}()

	return listener, nil
}

func (s *Server) wirePluginHooks() {
	mgr := s.deps.PluginManager
	if mgr == nil {
		return
	}

	s.pluginDone = make(chan struct{})

	s.wirePluginEventLoop("session.created", func(props map[string]any) {
		info, _ := props["info"].(map[string]any)
		if info == nil {
			return
		}
		sid, _ := info["id"].(string)
		if sid != "" {
			plugin.DispatchSessionStart(mgr, plugin.SessionStartEvent{SessionID: sid})
		}
	})

	s.wirePluginEventLoop("session.deleted", func(props map[string]any) {
		sid, _ := props["sessionID"].(string)
		if sid != "" {
			plugin.DispatchSessionEnd(mgr, plugin.SessionEndEvent{SessionID: sid})
		}
	})

	s.wirePluginEventLoop("permission.ask", func(props map[string]any) {
		sessionID, _ := props["sessionID"].(string)
		toolName, _ := props["toolName"].(string)
		toolArgs, _ := props["toolArgs"].(string)
		perm, _ := props["permission"].(string)
		plugin.DispatchPermissionAsk(mgr, plugin.PermissionInput{
			SessionID:  sessionID,
			ToolName:   toolName,
			ToolArgs:   toolArgs,
			Permission: perm,
		})
	})

	s.wirePluginEventLoop("shell.env", func(props map[string]any) {
		sessionID, _ := props["sessionID"].(string)
		directory, _ := props["directory"].(string)
		env, _ := props["env"].(map[string]string)
		plugin.DispatchShellEnv(mgr, plugin.ShellEnvInput{
			SessionID: sessionID,
			Directory: directory,
			Env:       env,
		})
	})

	s.wirePluginEventLoop("tool.execute.before", func(props map[string]any) {
		sessionID, _ := props["sessionID"].(string)
		toolName, _ := props["tool"].(string)
		toolArgs, _ := props["args"].(string)
		plugin.DispatchToolExecBefore(mgr, plugin.ToolExecBeforeEvent{
			SessionID: sessionID,
			ToolName:  toolName,
			ToolArgs:  toolArgs,
		})
	})

	// tool.execute.after dispatch is handled synchronously via tool.Context.AfterHook
	// to allow plugins to transform output before it's returned to the LLM.
}

// wirePluginEventLoop subscribes to a bus topic and runs the handler in a
// goroutine that exits when pluginDone is closed.
func (s *Server) wirePluginEventLoop(topic string, handler func(map[string]any)) {
	sub := s.deps.Bus.Subscribe(topic)
	s.pluginSubs = append(s.pluginSubs, sub)
	go func() {
		for {
			select {
			case <-s.pluginDone:
				return
			case evt, ok := <-sub.C:
				if !ok {
					return
				}
				props, ok := evt.Properties.(map[string]any)
				if !ok {
					continue
				}
				handler(props)
			}
		}
	}()
}

// WaitForShutdown blocks until the server's background shutdown goroutine
// (triggered by context cancellation in Listen) has finished draining
// all active session processors and closing the HTTP server.
func (s *Server) WaitForShutdown() {
	if s.shutdownDone != nil {
		<-s.shutdownDone
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	for _, sub := range s.pluginSubs {
		sub.Unsubscribe()
	}
	s.pluginSubs = nil
	return s.httpServer.Shutdown(ctx)
}
