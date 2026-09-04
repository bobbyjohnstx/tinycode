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
	"github.com/bobbyjohnstx/tinycode-go/internal/plugin"
	"github.com/bobbyjohnstx/tinycode-go/internal/provider"
	"github.com/bobbyjohnstx/tinycode-go/internal/server/middleware"
	"github.com/bobbyjohnstx/tinycode-go/internal/static"
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
		sessionManager:  NewSessionManager(deps.Bus, deps.Registry, deps.DB, cfg.Directory),
		permissionStore: NewPermissionStore(),
		questionStore:   NewQuestionStore(),
	}

	s.registerRoutes()

	if cfg.ServeWebUI {
		mux.Handle("/", static.Handler(cfg.WebUIDir))
	}

	corsConfig := middleware.DefaultCORSConfig()
	handler := middleware.CORS(corsConfig)(middleware.SecurityHeaders(mux))

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

	go func() {
		<-ctx.Done()
		s.logger.Info("shutting down server")
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

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
