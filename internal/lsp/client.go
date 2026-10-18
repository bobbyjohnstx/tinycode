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

	"github.com/bobbyjohnstx/tinycode/internal/procenv"
	"github.com/bobbyjohnstx/tinycode/internal/safego"
)

const (
	defaultRequestTimeout = 30 * time.Second
	maxOpenDocs           = 64
	maxStderrSize         = 64 * 1024
)

// cappedBuffer is a thread-safe writer that retains the last max bytes.
type cappedBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(p) >= b.max {
		b.buf = append(b.buf[:0], p[len(p)-b.max:]...)
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

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

// TextEdit is a textual edit applied to a document.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// WorkspaceEdit describes a set of changes across one or more documents.
// Supports both the changes map and documentChanges array shapes from LSP.
type WorkspaceEdit struct {
	Changes         map[string][]TextEdit `json:"changes,omitempty"`
	DocumentChanges []json.RawMessage     `json:"documentChanges,omitempty"`
}

// TextDocumentEdit is a documentChanges entry that edits an existing file.
type TextDocumentEdit struct {
	TextDocument VersionedTextDocumentIdentifier `json:"textDocument"`
	Edits        []TextEdit                      `json:"edits"`
}

// VersionedTextDocumentIdentifier identifies a document (version optional).
type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version *int   `json:"version,omitempty"`
}

// CodeAction is a language-server suggested edit or command (list-only; never applied).
type CodeAction struct {
	Title string         `json:"title"`
	Kind  string         `json:"kind,omitempty"`
	Edit  *WorkspaceEdit `json:"edit,omitempty"`
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

// DocumentSymbolNode is a hierarchical or flat document symbol for outlining.
type DocumentSymbolNode struct {
	Name          string
	Kind          int
	Line          int // 0-indexed selection/start line
	ContainerName string
	Children      []DocumentSymbolNode
	Flat          bool // true when parsed from SymbolInformation[]
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

	docMu     sync.Mutex
	openDocs  map[string]*docState
	openOrder []string // LRU: oldest at front

	diagMu      sync.Mutex
	diagStore   map[string][]Diagnostic
	diagWaiters map[string][]chan struct{}

	stderrBuf *cappedBuffer

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
		stderrBuf:   &cappedBuffer{max: maxStderrSize},
		done:        make(chan struct{}),
	}
}

// Alive reports whether the client connection is still live.
func (c *Client) Alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *Client) stderrSnippet() string {
	if c.stderrBuf == nil {
		return ""
	}
	return c.stderrBuf.String()
}

func (c *Client) connect(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.spec.Command, c.spec.Args...)
	cmd.Env = procenv.Child(c.env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("creating stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("creating stdout pipe: %w", err)
	}
	if c.stderrBuf == nil {
		c.stderrBuf = &cappedBuffer{max: maxStderrSize}
	}
	cmd.Stderr = c.stderrBuf

	if err := cmd.Start(); err != nil {
		if snippet := c.stderrSnippet(); snippet != "" {
			slog.Warn("lsp: server start failed", "language", c.spec.Language, "command", c.spec.Command, "error", err, "stderr", snippet)
		}
		return fmt.Errorf("starting %s: %w", c.spec.Command, err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.reader = bufio.NewReaderSize(stdout, 1024*1024)

	safego.Go(c.readLoop)

	// ConnectTimeout overrides only the initialize RPC wait. Never wrap ctx with
	// context.WithTimeout for the CommandContext/process lifetime.
	initTO := c.timeout
	if c.spec.ConnectTimeout > 0 {
		initTO = c.spec.ConnectTimeout
	}
	_, err = c.requestWithTimeout(ctx, initTO, "initialize", map[string]any{
		"processId": os.Getpid(),
		"rootUri":   fileURI(c.rootDir),
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"hover":          map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
				"definition":     map[string]any{},
				"references":     map[string]any{},
				"documentSymbol": map[string]any{},
				"codeAction":     map[string]any{},
				"rename":         map[string]any{},
				"publishDiagnostics": map[string]any{
					"relatedInformation": true,
				},
			},
			"workspace": map[string]any{
				"symbol":    map[string]any{},
				"applyEdit": true,
			},
		},
	})
	if err != nil {
		if snippet := c.stderrSnippet(); snippet != "" {
			slog.Warn("lsp: initialize failed", "language", c.spec.Language, "command", c.spec.Command, "error", err, "stderr", snippet)
		}
		_ = c.close()
		return fmt.Errorf("LSP initialize: %w", err)
	}

	if err := c.notify(ctx, "initialized", map[string]any{}); err != nil {
		if snippet := c.stderrSnippet(); snippet != "" {
			slog.Warn("lsp: initialized notify failed", "language", c.spec.Language, "command", c.spec.Command, "error", err, "stderr", snippet)
		}
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
	if snippet := c.stderrSnippet(); snippet != "" {
		slog.Warn("lsp: server closed with stderr", "language", c.spec.Language, "command", c.spec.Command, "stderr", snippet)
	}
	return nil
}

func (c *Client) readLoop() {
	defer func() {
		c.doneOnce.Do(func() { close(c.done) })
		if snippet := c.stderrSnippet(); snippet != "" {
			slog.Warn("lsp: server disconnected", "language", c.spec.Language, "command", c.spec.Command, "stderr", snippet)
		}
	}()

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

		// Server request (has both id and method) — respond to avoid blocking.
		// workspace/applyEdit is always refused (no disk writes from LSP path).
		if envelope.ID != nil && envelope.Method != "" {
			var result any
			if envelope.Method == "workspace/applyEdit" {
				result = map[string]any{"applied": false}
			}
			_ = c.writeMessage(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(*envelope.ID),
				"result":  result,
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

// diagnosticCounts returns aggregate error and warning counts from the store.
// Severity 1 = error, 2 = warning (LSP DiagnosticSeverity).
func (c *Client) diagnosticCounts() (errors, warnings int) {
	c.diagMu.Lock()
	defer c.diagMu.Unlock()
	for _, diags := range c.diagStore {
		for _, d := range diags {
			switch d.Severity {
			case 1:
				errors++
			case 2:
				warnings++
			}
		}
	}
	return errors, warnings
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
	return c.requestWithTimeout(ctx, c.timeout, method, params)
}

// requestWithTimeout waits for an LSP response using timeout instead of c.timeout.
// If timeout <= 0, falls back to c.timeout, then defaultRequestTimeout.
func (c *Client) requestWithTimeout(ctx context.Context, timeout time.Duration, method string, params any) (json.RawMessage, error) {
	if timeout <= 0 {
		timeout = c.timeout
	}
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}

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
	case <-time.After(timeout):
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("request timeout after %v", timeout)
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
		c.evictLRULocked(ctx)
		c.openDocs[uri] = &docState{version: 1, content: text}
		c.openOrder = append(c.openOrder, uri)
		return true, c.notify(ctx, "textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": langID,
				"version":    1,
				"text":       text,
			},
		})
	}

	c.touchOpenDocLocked(uri)
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

// evictLRULocked closes the least-recently-used open document until under the cap.
// Caller must hold docMu.
func (c *Client) evictLRULocked(ctx context.Context) {
	for len(c.openDocs) >= maxOpenDocs {
		if len(c.openOrder) == 0 {
			return
		}
		evictURI := c.openOrder[0]
		c.openOrder = c.openOrder[1:]
		delete(c.openDocs, evictURI)
		_ = c.notify(ctx, "textDocument/didClose", map[string]any{
			"textDocument": map[string]any{"uri": evictURI},
		})
		c.diagMu.Lock()
		delete(c.diagStore, evictURI)
		delete(c.diagWaiters, evictURI)
		c.diagMu.Unlock()
	}
}

// touchOpenDocLocked moves uri to the most-recently-used end of openOrder.
// Caller must hold docMu.
func (c *Client) touchOpenDocLocked(uri string) {
	for i, u := range c.openOrder {
		if u != uri {
			continue
		}
		c.openOrder = append(append(c.openOrder[:i:i], c.openOrder[i+1:]...), uri)
		return
	}
	c.openOrder = append(c.openOrder, uri)
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

// Diagnostics returns diagnostics for a file.
// When file content is unchanged, cached diagnostics are returned immediately.
// When content changed (or newly opened), waits up to 10s for publishDiagnostics.
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

	if !changed {
		c.diagMu.Lock()
		diags := c.diagStore[uri]
		c.diagMu.Unlock()
		result := make([]Diagnostic, len(diags))
		copy(result, diags)
		return result, nil
	}

	select {
	case <-ch:
	case <-time.After(10 * time.Second):
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

// DocumentSymbols returns document symbols for a file via textDocument/documentSymbol.
func (c *Client) DocumentSymbols(ctx context.Context, file string) ([]DocumentSymbolNode, error) {
	if _, err := c.syncFile(ctx, file); err != nil {
		return nil, err
	}

	result, err := c.request(ctx, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(file)},
	})
	if err != nil {
		return nil, err
	}
	return parseDocumentSymbols(result)
}

// CodeActions returns code actions for a range (list-only; never applied).
func (c *Client) CodeActions(ctx context.Context, path string, rng Range) ([]CodeAction, error) {
	if _, err := c.syncFile(ctx, path); err != nil {
		return nil, err
	}

	result, err := c.request(ctx, "textDocument/codeAction", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(path)},
		"range":        rng,
		"context":      map[string]any{"diagnostics": []any{}},
	})
	if err != nil {
		return nil, err
	}
	return parseCodeActions(result)
}

// Rename returns a WorkspaceEdit for renaming a symbol (never applied to disk).
func (c *Client) Rename(ctx context.Context, path string, pos Position, newName string) (*WorkspaceEdit, error) {
	if _, err := c.syncFile(ctx, path); err != nil {
		return nil, err
	}

	result, err := c.request(ctx, "textDocument/rename", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(path)},
		"position":     pos,
		"newName":      newName,
	})
	if err != nil {
		return nil, err
	}
	if string(result) == "null" || len(result) == 0 {
		return nil, nil
	}
	var edit WorkspaceEdit
	if err := json.Unmarshal(result, &edit); err != nil {
		return nil, err
	}
	return &edit, nil
}

func parseCodeActions(raw json.RawMessage) ([]CodeAction, error) {
	if string(raw) == "null" || len(raw) == 0 {
		return nil, nil
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, nil
	}
	out := make([]CodeAction, 0, len(items))
	for _, item := range items {
		var action CodeAction
		if err := json.Unmarshal(item, &action); err != nil {
			continue
		}
		// Command-only entries may lack title in rare cases; skip empty.
		if action.Title == "" {
			var cmd struct {
				Title string `json:"title"`
			}
			if err := json.Unmarshal(item, &cmd); err == nil {
				action.Title = cmd.Title
			}
		}
		if action.Title == "" {
			continue
		}
		out = append(out, action)
	}
	return out, nil
}

// fullDocumentRange returns a range covering the entire file content.
func fullDocumentRange(content string) Range {
	if content == "" {
		return Range{}
	}
	lines := strings.Split(content, "\n")
	lastLine := len(lines) - 1
	return Range{
		Start: Position{Line: 0, Character: 0},
		End:   Position{Line: lastLine, Character: len(lines[lastLine])},
	}
}

func parseDocumentSymbols(raw json.RawMessage) ([]DocumentSymbolNode, error) {
	if string(raw) == "null" || len(raw) == 0 {
		return nil, nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, nil
	}
	if len(items) == 0 {
		return nil, nil
	}

	// SymbolInformation[] elements have "location" and lack hierarchical DocumentSymbol shape.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(items[0], &probe); err != nil {
		return nil, nil
	}
	_, hasLocation := probe["location"]
	_, hasChildren := probe["children"]
	if hasLocation && !hasChildren {
		return parseSymbolInformationList(items)
	}
	return parseDocumentSymbolTree(items)
}

func parseSymbolInformationList(items []json.RawMessage) ([]DocumentSymbolNode, error) {
	out := make([]DocumentSymbolNode, 0, len(items))
	for _, item := range items {
		var si struct {
			Name          string   `json:"name"`
			Kind          int      `json:"kind"`
			Location      Location `json:"location"`
			ContainerName string   `json:"containerName,omitempty"`
		}
		if err := json.Unmarshal(item, &si); err != nil {
			continue
		}
		out = append(out, DocumentSymbolNode{
			Name:          si.Name,
			Kind:          si.Kind,
			Line:          si.Location.Range.Start.Line,
			ContainerName: si.ContainerName,
			Flat:          true,
		})
	}
	return out, nil
}

func parseDocumentSymbolTree(items []json.RawMessage) ([]DocumentSymbolNode, error) {
	out := make([]DocumentSymbolNode, 0, len(items))
	for _, item := range items {
		node, ok := parseOneDocumentSymbol(item)
		if !ok {
			continue
		}
		out = append(out, node)
	}
	return out, nil
}

func parseOneDocumentSymbol(raw json.RawMessage) (DocumentSymbolNode, bool) {
	var ds struct {
		Name           string            `json:"name"`
		Kind           int               `json:"kind"`
		Range          *Range            `json:"range"`
		SelectionRange *Range            `json:"selectionRange"`
		Children       []json.RawMessage `json:"children"`
	}
	if err := json.Unmarshal(raw, &ds); err != nil || ds.Name == "" {
		return DocumentSymbolNode{}, false
	}
	line := 0
	switch {
	case ds.SelectionRange != nil:
		line = ds.SelectionRange.Start.Line
	case ds.Range != nil:
		line = ds.Range.Start.Line
	}
	node := DocumentSymbolNode{
		Name: ds.Name,
		Kind: ds.Kind,
		Line: line,
	}
	for _, child := range ds.Children {
		if cn, ok := parseOneDocumentSymbol(child); ok {
			node.Children = append(node.Children, cn)
		}
	}
	return node, true
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
