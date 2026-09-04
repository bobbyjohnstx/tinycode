package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/bobbyjohnstx/tinycode-go/internal/bus"
	"github.com/bobbyjohnstx/tinycode-go/internal/config"
	"github.com/bobbyjohnstx/tinycode-go/internal/tool"
)

type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusConnected    Status = "connected"
	StatusError        Status = "error"
)

type ServerStatus struct {
	Name      string `json:"name"`
	Status    Status `json:"status"`
	Error     string `json:"error,omitempty"`
	ToolCount int    `json:"toolCount"`
}

type MCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

type Service struct {
	mu      sync.RWMutex
	servers map[string]*serverConn
	bus     *bus.Bus
}

type serverConn struct {
	name      string
	config    config.MCPConfig
	transport Transport
	status    Status
	err       string
	tools     []MCPTool
	cancel    context.CancelFunc
}

func NewService(b *bus.Bus) *Service {
	return &Service{
		servers: make(map[string]*serverConn),
		bus:     b,
	}
}

func (s *Service) Configure(ctx context.Context, mcpConfigs map[string]config.MCPConfig) {
	s.mu.Lock()

	for name := range s.servers {
		if _, exists := mcpConfigs[name]; !exists {
			conn := s.servers[name]
			delete(s.servers, name)
			s.mu.Unlock()
			s.stopServer(conn)
			s.mu.Lock()
		}
	}

	for name, cfg := range mcpConfigs {
		if _, exists := s.servers[name]; !exists {
			s.servers[name] = &serverConn{
				name:   name,
				config: cfg,
				status: StatusDisconnected,
			}
		}
	}
	s.mu.Unlock()

	for name := range mcpConfigs {
		go s.connectServer(ctx, name)
	}
}

func (s *Service) connectServer(ctx context.Context, name string) {
	s.mu.Lock()
	conn, ok := s.servers[name]
	if !ok {
		s.mu.Unlock()
		return
	}
	conn.status = StatusConnecting
	conn.err = ""
	s.mu.Unlock()

	s.publishStatus(name)

	transport, err := s.createTransport(conn.config)
	if err != nil {
		s.mu.Lock()
		conn.status = StatusError
		conn.err = err.Error()
		s.mu.Unlock()
		s.publishStatus(name)
		return
	}

	connCtx, cancel := context.WithCancel(ctx)
	if err := transport.Connect(connCtx); err != nil {
		cancel()
		s.mu.Lock()
		conn.status = StatusError
		conn.err = err.Error()
		s.mu.Unlock()
		s.publishStatus(name)
		return
	}

	tools, err := transport.ListTools(connCtx)
	if err != nil {
		cancel()
		_ = transport.Close()
		s.mu.Lock()
		conn.status = StatusError
		conn.err = fmt.Sprintf("listing tools: %v", err)
		s.mu.Unlock()
		s.publishStatus(name)
		return
	}

	s.mu.Lock()
	conn.transport = transport
	conn.cancel = cancel
	conn.tools = tools
	conn.status = StatusConnected
	conn.err = ""
	s.mu.Unlock()

	s.publishStatus(name)
	slog.Info("mcp server connected", "name", name, "tools", len(tools))
}

func (s *Service) createTransport(cfg config.MCPConfig) (Transport, error) {
	transport := resolveTransport(cfg)
	switch transport {
	case "stdio":
		if cfg.Command == "" {
			return nil, fmt.Errorf("stdio transport requires command")
		}
		return NewStdioTransport(cfg.Command, cfg.Args, cfg.Env), nil
	case "sse":
		if cfg.URL == "" {
			return nil, fmt.Errorf("SSE transport requires url")
		}
		return NewSSETransport(cfg.URL, cfg.Headers), nil
	case "streamable-http":
		if cfg.URL == "" {
			return nil, fmt.Errorf("streamable-http transport requires url")
		}
		return NewStreamableHTTPTransport(cfg.URL, cfg.Headers), nil
	default:
		return nil, fmt.Errorf("unknown transport: %s", transport)
	}
}

func resolveTransport(cfg config.MCPConfig) string {
	if cfg.Transport != "" {
		return cfg.Transport
	}
	if cfg.Command != "" {
		return "stdio"
	}
	if cfg.URL != "" {
		return "sse"
	}
	return "stdio"
}

func (s *Service) stopServer(conn *serverConn) {
	if conn.cancel != nil {
		conn.cancel()
	}
	if conn.transport != nil {
		if err := conn.transport.Close(); err != nil {
			slog.Warn("error closing mcp transport", "name", conn.name, "error", err)
		}
	}
}

func (s *Service) Tools(ctx context.Context) map[string]*tool.Def {
	s.mu.RLock()
	defer s.mu.RUnlock()

	defs := make(map[string]*tool.Def)
	for serverName, conn := range s.servers {
		if conn.status != StatusConnected {
			continue
		}
		for _, t := range conn.tools {
			toolID := fmt.Sprintf("mcp__%s__%s", serverName, t.Name)
			defs[toolID] = convertMCPTool(serverName, t, conn.transport)
		}
	}
	return defs
}

func (s *Service) Status(_ context.Context) map[string]ServerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]ServerStatus, len(s.servers))
	for name, conn := range s.servers {
		result[name] = ServerStatus{
			Name:      name,
			Status:    conn.status,
			Error:     conn.err,
			ToolCount: len(conn.tools),
		}
	}
	return result
}

func (s *Service) Restart(ctx context.Context, name string) error {
	s.mu.Lock()
	conn, ok := s.servers[name]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("unknown MCP server: %s", name)
	}
	s.mu.Unlock()

	s.stopServer(conn)

	s.mu.Lock()
	conn.status = StatusDisconnected
	conn.transport = nil
	conn.cancel = nil
	conn.tools = nil
	s.mu.Unlock()

	go s.connectServer(ctx, name)
	return nil
}

func (s *Service) Close() {
	s.mu.Lock()
	servers := make([]*serverConn, 0, len(s.servers))
	for _, conn := range s.servers {
		servers = append(servers, conn)
	}
	s.servers = make(map[string]*serverConn)
	s.mu.Unlock()

	for _, conn := range servers {
		s.stopServer(conn)
	}
}

func (s *Service) publishStatus(name string) {
	s.mu.RLock()
	conn, ok := s.servers[name]
	if !ok {
		s.mu.RUnlock()
		return
	}
	status := ServerStatus{
		Name:      name,
		Status:    conn.status,
		Error:     conn.err,
		ToolCount: len(conn.tools),
	}
	s.mu.RUnlock()

	s.bus.Publish("mcp.status", map[string]any{
		"server": status,
	})
}

func convertMCPTool(serverName string, t MCPTool, transport Transport) *tool.Def {
	toolID := fmt.Sprintf("mcp__%s__%s", serverName, t.Name)
	mcpToolName := t.Name

	var params map[string]any
	if len(t.InputSchema) > 0 {
		_ = json.Unmarshal(t.InputSchema, &params)
	}
	if params == nil {
		params = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}

	return &tool.Def{
		ID:          toolID,
		Description: t.Description,
		Parameters:  params,
		Permission:  "mcp",
		Execute: func(ctx context.Context, tc *tool.Context, args json.RawMessage) (*tool.ExecuteResult, error) {
			result, err := transport.CallTool(ctx, mcpToolName, args)
			if err != nil {
				return &tool.ExecuteResult{Output: err.Error(), IsError: true}, nil
			}
			return &tool.ExecuteResult{Output: result, IsError: false}, nil
		},
	}
}
