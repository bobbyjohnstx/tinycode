package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const defaultRequestTimeout = 30 * time.Second

// Position in a text document (0-indexed, per LSP spec).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range in a text document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location links a range to a document URI.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// Diagnostic represents a compiler error or warning.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
	Code     any    `json:"code,omitempty"`
}

// SymbolInfo represents a workspace symbol.
type SymbolInfo struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

type lspResponse struct {
	Result json.RawMessage
	Error  *jsonrpcError
}

type docState struct {
	version int
	content string
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Client manages a single LSP server process.
type Client struct {
	spec    ServerSpec
	rootDir string
	env     map[string]string
	timeout time.Duration

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader
	nextID atomic.Int64

	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[int64]chan lspResponse

	docMu    sync.Mutex
	openDocs map[string]*docState

	diagMu      sync.Mutex
	diagStore   map[string][]Diagnostic
	diagWaiters map[string][]chan struct{}

	done     chan struct{}
	doneOnce sync.Once
}

func newClient(spec ServerSpec, rootDir string, env map[string]string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = defaultRequestTimeout
	}
	return &Client{
		spec:        spec,
		rootDir:     rootDir,
		env:         env,
		timeout:     timeout,
		pending:     make(map[int64]chan lspResponse),
		openDocs:    make(map[string]*docState),
		diagStore:   make(map[string][]Diagnostic),
		diagWaiters: make(map[string][]chan struct{}),
		done:        make(chan struct{}),
	}
}

func (c *Client) connect(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.spec.Command, c.spec.Args...)
	cmd.Env = os.Environ()
	for k, v := range c.env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("creating stdout pipe: %w", err)
	}
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", c.spec.Command, err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.reader = bufio.NewReaderSize(stdout, 1024*1024)

	safego.Go(c.readLoop)

	_, err = c.request(ctx, "initialize", map[string]any{
		"processId": os.Getpid(),
		"rootUri":   fileURI(c.rootDir),
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"synchronization": map[string]any{"didSave": true},
				"hover":           map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
				"definition":      map[string]any{},
				"references":      map[string]any{},
				"documentSymbol":  map[string]any{},
				"publishDiagnostics": map[string]any{
					"relatedInformation": true,
				},
			},
			"workspace": map[string]any{
				"symbol": map[string]any{},
			},
		},
	})
	if err != nil {
		_ = c.close()
		return fmt.Errorf("LSP initialize: %w", err)
	}

	if err := c.notify(ctx, "initialized", map[string]any{}); err != nil {
		_ = c.close()
		return fmt.Errorf("sending initialized: %w", err)
	}

	slog.Info("lsp server connected", "language", c.spec.Language, "command", c.spec.Command)
	return nil
}

func (c *Client) close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _ = c.request(ctx, "shutdown", nil)
	_ = c.notify(ctx, "exit", nil)

	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		done := make(chan error, 1)
		safego.Go(func() { done <- c.cmd.Wait() })
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = c.cmd.Process.Kill()
			<-done
		}
	}
	c.doneOnce.Do(func() { close(c.done) })
	return nil
}

func (c *Client) readLoop() {
	defer c.doneOnce.Do(func() { close(c.done) })

	for {
		msg, err := c.readMessage()
		if err != nil {
			return
		}

		var envelope struct {
			ID     *json.RawMessage `json:"id,omitempty"`
			Method string           `json:"method,omitempty"`
			Result json.RawMessage  `json:"result,omitempty"`
			Error  *jsonrpcError    `json:"error,omitempty"`
			Params json.RawMessage  `json:"params,omitempty"`
		}
		if err := json.Unmarshal(msg, &envelope); err != nil {
			continue
		}

		// Server request (has both id and method) — respond to avoid blocking
		if envelope.ID != nil && envelope.Method != "" {
			_ = c.writeMessage(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(*envelope.ID),
				"result":  nil,
			})
			continue
		}

		// Response to a request we sent
		if envelope.ID != nil {
			var id int64
			if err := json.Unmarshal(*envelope.ID, &id); err != nil {
				continue
			}

			resp := lspResponse{Result: envelope.Result, Error: envelope.Error}

			c.pendingMu.Lock()
			ch, ok := c.pending[id]
			delete(c.pending, id)
			c.pendingMu.Unlock()

			if ok {
				select {
				case ch <- resp:
				default:
				}
			}
			continue
		}

		// Notification from server
		if envelope.Method == "textDocument/publishDiagnostics" {
			c.handlePublishDiagnostics(envelope.Params)
		}
	}
}

func (c *Client) handlePublishDiagnostics(params json.RawMessage) {
	var p struct {
		URI         string       `json:"uri"`
		Diagnostics []Diagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return
	}

	c.diagMu.Lock()
	c.diagStore[p.URI] = p.Diagnostics
	waiters := c.diagWaiters[p.URI]
	c.diagWaiters[p.URI] = nil
	c.diagMu.Unlock()

	for _, ch := range waiters {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (c *Client) readMessage() (json.RawMessage, error) {
	var contentLength int
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			n, err := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			if err == nil {
				contentLength = n
			}
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(c.reader, body); err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

func (c *Client) writeMessage(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))

	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return err
	}
	_, err = c.stdin.Write(data)
	return err
}

func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := c.nextID.Add(1)

	ch := make(chan lspResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}

	if err := c.writeMessage(req); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("LSP error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-time.After(c.timeout):
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("request timeout after %v", c.timeout)
	case <-c.done:
		return nil, fmt.Errorf("server disconnected")
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *Client) notify(_ context.Context, method string, params any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		msg["params"] = params
	}
	return c.writeMessage(msg)
}

// syncFile ensures the LSP server has the current content of file.
// Returns true if the file was opened or content changed.
func (c *Client) syncFile(ctx context.Context, file string) (bool, error) {
	content, err := os.ReadFile(file)
	if err != nil {
		return false, fmt.Errorf("reading %s: %w", file, err)
	}

	uri := fileURI(file)
	langID := languageIDForFile(file)
	text := string(content)

	c.docMu.Lock()
	defer c.docMu.Unlock()

	doc, exists := c.openDocs[uri]
	if !exists {
		c.openDocs[uri] = &docState{version: 1, content: text}
		return true, c.notify(ctx, "textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": langID,
				"version":    1,
				"text":       text,
			},
		})
	}

	if doc.content == text {
		return false, nil
	}

	doc.version++
	doc.content = text
	return true, c.notify(ctx, "textDocument/didChange", map[string]any{
		"textDocument": map[string]any{
			"uri":     uri,
			"version": doc.version,
		},
		"contentChanges": []map[string]any{
			{"text": text},
		},
	})
}

// Hover returns hover information for a position (0-indexed line and column).
func (c *Client) Hover(ctx context.Context, file string, line, col int) (string, error) {
	if _, err := c.syncFile(ctx, file); err != nil {
		return "", err
	}

	result, err := c.request(ctx, "textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(file)},
		"position":     Position{Line: line, Character: col},
	})
	if err != nil {
		return "", err
	}

	if string(result) == "null" || len(result) == 0 {
		return "", nil
	}

	var hover struct {
		Contents json.RawMessage `json:"contents"`
	}
	if err := json.Unmarshal(result, &hover); err != nil {
		return "", err
	}
	return parseHoverContents(hover.Contents), nil
}

// Definition returns definition location(s) for a position (0-indexed).
func (c *Client) Definition(ctx context.Context, file string, line, col int) ([]Location, error) {
	if _, err := c.syncFile(ctx, file); err != nil {
		return nil, err
	}

	result, err := c.request(ctx, "textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(file)},
		"position":     Position{Line: line, Character: col},
	})
	if err != nil {
		return nil, err
	}
	return parseLocations(result)
}

// References returns all reference locations for a position (0-indexed).
func (c *Client) References(ctx context.Context, file string, line, col int, includeDecl bool) ([]Location, error) {
	if _, err := c.syncFile(ctx, file); err != nil {
		return nil, err
	}

	result, err := c.request(ctx, "textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(file)},
		"position":     Position{Line: line, Character: col},
		"context":      map[string]any{"includeDeclaration": includeDecl},
	})
	if err != nil {
		return nil, err
	}
	return parseLocations(result)
}

// Diagnostics returns diagnostics for a file, waiting for the server to produce them.
func (c *Client) Diagnostics(ctx context.Context, file string) ([]Diagnostic, error) {
	uri := fileURI(file)

	ch := make(chan struct{}, 1)
	c.diagMu.Lock()
	c.diagWaiters[uri] = append(c.diagWaiters[uri], ch)
	c.diagMu.Unlock()
	defer c.removeDiagWaiter(uri, ch)

	changed, err := c.syncFile(ctx, file)
	if err != nil {
		return nil, err
	}

	timeout := 500 * time.Millisecond
	if changed {
		timeout = 10 * time.Second
	}

	select {
	case <-ch:
	case <-time.After(timeout):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	c.diagMu.Lock()
	diags := c.diagStore[uri]
	c.diagMu.Unlock()

	result := make([]Diagnostic, len(diags))
	copy(result, diags)
	return result, nil
}

func (c *Client) removeDiagWaiter(uri string, ch chan struct{}) {
	c.diagMu.Lock()
	defer c.diagMu.Unlock()
	waiters := c.diagWaiters[uri]
	kept := waiters[:0]
	found := false
	for _, w := range waiters {
		if !found && w == ch {
			found = true
			continue
		}
		kept = append(kept, w)
	}
	if !found {
		return
	}
	if len(kept) == 0 {
		delete(c.diagWaiters, uri)
		return
	}
	c.diagWaiters[uri] = kept
}

// Symbols searches for workspace symbols matching query.
func (c *Client) Symbols(ctx context.Context, query string) ([]SymbolInfo, error) {
	result, err := c.request(ctx, "workspace/symbol", map[string]any{
		"query": query,
	})
	if err != nil {
		return nil, err
	}

	var symbols []SymbolInfo
	if err := json.Unmarshal(result, &symbols); err != nil {
		return nil, err
	}
	return symbols, nil
}

func parseHoverContents(raw json.RawMessage) string {
	var mc struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &mc); err == nil && mc.Value != "" {
		return mc.Value
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		var parts []string
		for _, item := range arr {
			var ms struct {
				Language string `json:"language"`
				Value    string `json:"value"`
			}
			if err := json.Unmarshal(item, &ms); err == nil && ms.Value != "" {
				if ms.Language != "" {
					parts = append(parts, fmt.Sprintf("```%s\n%s\n```", ms.Language, ms.Value))
				} else {
					parts = append(parts, ms.Value)
				}
				continue
			}
			var str string
			if err := json.Unmarshal(item, &str); err == nil {
				parts = append(parts, str)
			}
		}
		return strings.Join(parts, "\n\n")
	}

	return string(raw)
}

func parseLocations(raw json.RawMessage) ([]Location, error) {
	if string(raw) == "null" || len(raw) == 0 {
		return nil, nil
	}

	var single Location
	if err := json.Unmarshal(raw, &single); err == nil && single.URI != "" {
		return []Location{single}, nil
	}

	var arr []Location
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}

	var links []struct {
		TargetURI   string `json:"targetUri"`
		TargetRange Range  `json:"targetRange"`
	}
	if err := json.Unmarshal(raw, &links); err == nil {
		locs := make([]Location, len(links))
		for i, l := range links {
			locs[i] = Location{URI: l.TargetURI, Range: l.TargetRange}
		}
		return locs, nil
	}

	return nil, nil
}

func fileURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	// url.URL.String percent-encodes Path (spaces, #, etc.).
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	return u.String()
}

func fileFromURI(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return strings.TrimPrefix(uri, "file://")
	}
	return filepath.FromSlash(u.Path)
}
