package lsp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeLSPScript is a minimal Python LSP server for unit tests.
// Env:
//
//	FAKE_LSP_LOG — append JSON lines for each inbound message
//	FAKE_LSP_DIAG_DELAY — seconds to wait after didOpen/didChange before publishDiagnostics
const fakeLSPScript = `
import json, os, sys, time

def read_msg():
    headers = {}
    while True:
        line = sys.stdin.buffer.readline()
        if not line:
            return None
        line = line.decode("utf-8").rstrip("\r\n")
        if line == "":
            break
        if ":" in line:
            k, v = line.split(":", 1)
            headers[k.strip().lower()] = v.strip()
    n = int(headers.get("content-length", "0"))
    if n <= 0:
        return None
    body = sys.stdin.buffer.read(n)
    return json.loads(body.decode("utf-8"))

def write_msg(obj):
    data = json.dumps(obj).encode("utf-8")
    sys.stdout.buffer.write(("Content-Length: %d\r\n\r\n" % len(data)).encode("utf-8"))
    sys.stdout.buffer.write(data)
    sys.stdout.buffer.flush()

def log(obj):
    path = os.environ.get("FAKE_LSP_LOG")
    if not path:
        return
    with open(path, "a") as f:
        f.write(json.dumps(obj) + "\n")

diag_delay = float(os.environ.get("FAKE_LSP_DIAG_DELAY", "0") or "0")
init_delay = float(os.environ.get("FAKE_LSP_INIT_DELAY", "0") or "0")
init_hang = os.environ.get("FAKE_LSP_INIT_HANG", "") == "1"
hang_after_init = os.environ.get("FAKE_LSP_HANG_AFTER_INIT", "") == "1"

while True:
    msg = read_msg()
    if msg is None:
        break
    log(msg)
    method = msg.get("method")
    mid = msg.get("id")
    if method == "initialize":
        if init_hang:
            while True:
                time.sleep(3600)
        if init_delay:
            time.sleep(init_delay)
        write_msg({
            "jsonrpc": "2.0",
            "id": mid,
            "result": {
                "capabilities": {
                    "textDocumentSync": 1,
                    "hoverProvider": True,
                    "definitionProvider": True,
                    "referencesProvider": True,
                    "documentSymbolProvider": True,
                    "workspaceSymbolProvider": True,
                }
            },
        })
    elif method == "shutdown":
        write_msg({"jsonrpc": "2.0", "id": mid, "result": None})
    elif method == "exit":
        break
    elif method in ("textDocument/didOpen", "textDocument/didChange"):
        params = msg.get("params") or {}
        td = params.get("textDocument") or {}
        uri = td.get("uri", "")
        if uri:
            if diag_delay:
                time.sleep(diag_delay)
            write_msg({
                "jsonrpc": "2.0",
                "method": "textDocument/publishDiagnostics",
                "params": {
                    "uri": uri,
                    "diagnostics": [{
                        "range": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 1}},
                        "severity": 1,
                        "message": "fake error",
                    }],
                },
            })
    elif method == "textDocument/documentSymbol" and mid is not None:
        write_msg({
            "jsonrpc": "2.0",
            "id": mid,
            "result": [{
                "name": "Foo",
                "kind": 5,
                "range": {"start": {"line": 0, "character": 0}, "end": {"line": 10, "character": 0}},
                "selectionRange": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 3}},
                "children": [{
                    "name": "bar",
                    "kind": 6,
                    "range": {"start": {"line": 2, "character": 0}, "end": {"line": 4, "character": 0}},
                    "selectionRange": {"start": {"line": 2, "character": 4}, "end": {"line": 2, "character": 7}},
                    "children": [],
                }],
            }],
        })
    elif mid is not None:
        if hang_after_init:
            while True:
                time.sleep(3600)
        write_msg({"jsonrpc": "2.0", "id": mid, "result": None})
`

func writeFakeLSPServer(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake_lsp.py")
	if err := os.WriteFile(path, []byte(fakeLSPScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func fakeLSPSpec(script string) ServerSpec {
	return ServerSpec{
		Language:   "go",
		Command:    "python3",
		Args:       []string{script},
		ExtLangIDs: map[string]string{".go": "go"},
	}
}

func readFakeLSPLog(t *testing.T, logPath string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("parse log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func waitForFakeLogMethod(t *testing.T, logPath, method string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, msg := range readFakeLSPLog(t, logPath) {
			if msg["method"] == method {
				return msg
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for method %q in %s", method, logPath)
	return nil
}

func countFakeLogMethod(t *testing.T, logPath, method string) int {
	t.Helper()
	n := 0
	for _, msg := range readFakeLSPLog(t, logPath) {
		if msg["method"] == method {
			n++
		}
	}
	return n
}

func writeGoFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func connectFakeClient(t *testing.T, script string, root string, env map[string]string) *Client {
	t.Helper()
	if root == "" {
		root = t.TempDir()
	}
	client := newClient(fakeLSPSpec(script), root, env, 5*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	if err := client.connect(ctx); err != nil {
		t.Fatalf("connect fake lsp: %v", err)
	}
	t.Cleanup(func() { _ = client.close() })
	return client
}
