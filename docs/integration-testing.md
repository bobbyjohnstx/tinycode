# Integration Testing

tinycode has two integration test suites: **headless** (run-mode) and **TUI** (tmux-based). Both live in `script/` and are shell scripts that exercise the built binary end-to-end against a real Ollama model.

## Prerequisites

| Dependency | Required by | Install |
|---|---|---|
| Go 1.27.1+ | `make build` | `brew install go` |
| tmux | TUI tests | `brew install tmux` |
| jq | Headless tests | `brew install jq` |
| sqlite3 | TC-H18 (session title) | pre-installed on macOS |
| Ollama | Both suites | [ollama.com](https://ollama.com) |

Before running either suite:

```bash
make build                          # produces dist/tinycode
ollama pull qwen3.5:9b              # default test model
```

## Headless Tests

**Script:** `script/test-headless.sh`

Exercises the `tinycode run` headless mode — single-prompt and multi-turn, NDJSON and plain-text output, tool execution, permissions, sessions, error recovery.

### Usage

```bash
./script/test-headless.sh              # all 19 tests
./script/test-headless.sh TC-H01       # single test
./script/test-headless.sh --list       # list test IDs and names
```

### Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `TINYCODE_TEST_MODEL` | `ollama/qwen3.5:9b` | Model to use for all tests |
| `TINYCODE_TEST_TIMEOUT` | `120` | Per-test timeout in seconds |
| `TINYCODE` | `./dist/tinycode` | Path to the built binary |

### Test Inventory

| ID | Name | What it validates |
|---|---|---|
| TC-H01 | Basic prompt response | Stdin prompt produces text + step_start events |
| TC-H02 | NDJSON event types | Tool call produces step_start/finish, tool_begin/end |
| TC-H03 | Tool execution | Shell tool runs `echo` and produces tool_end |
| TC-H06 | Multi-turn conversation | JSON input protocol with `{"type":"prompt"}` and `{"type":"exit"}` |
| TC-H06b | Multi-turn with tool use | Cross-turn state: turn 1 creates file, turn 2 reads it |
| TC-H07 | Max iterations cap | `--max-iterations 2` limits step_start count |
| TC-H09 | Agent selection | `--agent architect` selects a non-default agent |
| TC-H11 | Invalid model error | Bad model name exits non-zero |
| TC-H12 | Stdin pipe input | Plain-text (non-JSON) output via pipe |
| TC-H13 | Session continuation | `--continue` flag reloads tool-call history from SQLite |
| TC-H14 | JSON permission protocol | `--permissions json` emits structured permission events |
| TC-H15 | Tool error recovery | Model retries after tool returns an error |
| TC-H16 | File tool usage (read) | Verifies `read` tool (not shell) is used |
| TC-H17 | Default format output | Non-JSON output with no `--format` flag |
| TC-H18 | Session title | `--title` stores title in SQLite DB |
| TC-H19 | Multiple tool calls | Two sequential shell tool calls |
| TC-H20 | Grep tool usage | `grep` tool (not shell) finds a pattern |
| TC-H21 | SIGINT graceful exit | SIGINT mid-stream exits cleanly (code 0, 1, or 130) |
| TC-H22 | Glob tool usage | `glob` tool (not shell) finds .txt files |

### Writing New Headless Tests

Each test is a bash function named `test_hNN_description()`. The pattern:

1. Run `tinycode run --format json --dangerously-skip-permissions -m "$MODEL"` with a prompt
2. Capture NDJSON output to a file
3. Assert against the output using the built-in helpers

Available assertion helpers:

```bash
assert_exit_zero $rc "label"                    # exit code == 0
assert_exit_nonzero $rc "label"                 # exit code != 0
assert_output_contains "$output" "needle" "label"  # string search in output
assert_ndjson_has_type "$file" "text" "label"   # NDJSON contains event type
assert_ndjson_type_count_ge "$file" "tool_begin" 2 "label"  # event count >= N
assert_ndjson_type_count_le "$file" "step_start" 2 "label"  # event count <= N
assert_file_contains "$file" "pattern" "label"  # grep in a file
```

After adding the function, register it in `main()`:

```bash
[ -z "$filter" ] || [ "$filter" = "TC-HNN" ] && run_test "TC-HNN" "Description" test_hNN_description
```

### NDJSON Event Types

The `--format json` output emits one JSON object per line with these `type` values:

| Type | When | Key fields |
|---|---|---|
| `step_start` | LLM call begins | `stepIndex` |
| `step_finish` | LLM call ends | `stepIndex` |
| `text` | Text delta from LLM | `text` |
| `reasoning` | Reasoning/thinking delta | `text` |
| `tool_begin` | Tool call starts | `toolName`, `toolCallID` |
| `tool_call_end` | LLM finished tool-call args | `toolName`, `toolCallID`, `toolArgs` |
| `tool_end` | Tool execution finishes | `toolName`, `toolCallID`, `output`, `isError` |
| `ready` | Multi-turn: ready for next input | — |
| `permission` | Permission request (with `--permissions json`) | `id`, `permission`, `metadata` |
| `compacted` | Context compaction occurred | — |

## TUI Tests

**Script:** `script/tui-test.sh`

Exercises the Bubbletea TUI via tmux sessions — starts tinycode in a virtual terminal, sends keystrokes, captures the pane, and asserts on rendered content.

### Usage

```bash
./script/tui-test.sh               # all 30 tests
./script/tui-test.sh T01           # single test
./script/tui-test.sh T01 T05 T10   # multiple specific tests
```

### Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `TINYCODE_BIN` | `./dist/tinycode` | Path to the built binary |

### How It Works

Each test:

1. Creates a tmux session at a specific size (default 120x40)
2. Launches `tinycode` with `TINYCODE_DISABLE_MOUSE=1 TINYCODE_DB=:memory:`
3. Waits 3 seconds for startup (covers the 2-second startup guard that discards rune input)
4. Sends keystrokes via `tmux send-keys`
5. Captures the rendered pane via `tmux capture-pane -p`
6. Asserts text is present, absent, or matches a regex
7. Kills the tmux session

### Test Inventory

**Core UI (no LLM required):**

| ID | What it tests |
|---|---|
| T01 | TUI renders on startup |
| T02 | Ctrl+D and /exit both exit cleanly (two assertions) |
| T03 | Ctrl+C clears prompt text |
| T04 | Ctrl+P opens command palette |
| T05 | Leader+b toggles sidebar |
| T06 | Tab/Shift+Tab cycle agent in status bar |
| T07 | Leader+n creates new session |
| T08 | Leader+m shows model list |
| T09 | Terminal resize handling |
| T10 | Slash command autocomplete popup |
| T11 | Escape dismisses palette |
| T12 | Leader key timeout (500ms expiry) |
| T13 | Welcome screen ("Getting Started") |
| T14 | Text input renders in prompt area |
| T15 | Sidebar hides at narrow width (80 cols < threshold 120) |
| T16 | Multiple Ctrl+C doesn't crash (no panic/goroutine trace) |
| T17 | Resize with sidebar open auto-hides at narrow width |
| T18 | Shift+Enter inserts newline (multiline prompt) |
| T19 | Leader+a opens agent list dialog |
| T20 | Leader+o opens session list dialog |
| T21 | Status bar shows hint keywords ("agents", "commands") |
| T22 | Prompt metadata line renders (separator or "No provider") |
| T23 | Escape is no-op when idle |
| T24 | Up arrow recalls prompt history |
| T25 | Palette type-to-filter ("con" shows connect, hides theme) |
| T26 | Agent dialog Escape dismisses |
| T33 | Very small terminal (40x10) crash resistance |

**LLM-connected (require Ollama with `qwen3.5:9b`):**

| ID | What it tests |
|---|---|
| T28 | LLM response rendering: welcome screen replaced, agent footer visible |
| T29 | Thought block toggle: T key expands/collapses reasoning blocks (skips if model lacks reasoning) |
| T30 | Permission prompt: shell tool triggers permission overlay, Enter approves |

### Writing New TUI Tests

Each test is a bash function named `test_TNN()`. Template:

```bash
test_T99() {
    echo -e "${BOLD}T99: Description of test${NC}"
    local session
    session=$(new_session "T99")      # or new_session_with_model for LLM tests
    sleep 3                           # wait for startup guard

    send_keys "$session" "some input"
    sleep 1

    assert_contains "$session" "expected text" "assertion label"

    kill_session "$session"
}
```

Available helpers:

```bash
# Session management
new_session "T99"                           # 120x40, no model
new_session_with_model "T99"                # 120x40, with ollama/qwen3.5:9b

# Input
send_keys "$session" "text"                 # raw text (no Enter)
send_keys "$session" C-p                    # Ctrl+P
send_keys "$session" C-x                    # Leader key
send_keys "$session" Tab                    # Tab
send_keys "$session" BTab                   # Shift+Tab
send_keys "$session" Escape                 # Escape
send_keys "$session" Enter                  # Enter
send_keys "$session" S-Enter                # Shift+Enter
send_keys "$session" Up                     # Arrow up
send_text "$session" "text"                 # text + Enter

# Waiting
wait_for_text "$session" "pattern" 30       # poll until text appears (timeout seconds)
wait_for_regex "$session" "pat.*ern" 30     # poll until regex matches
wait_response_complete "$session" 120       # wait for LLM response (spinner appears then disappears)

# Assertions (each increments PASS/FAIL/TOTAL counters)
assert_contains "$session" "text" "label"       # pane contains literal text
assert_not_contains "$session" "text" "label"   # pane does NOT contain text
assert_regex "$session" "pat.*ern" "label"      # pane matches extended regex
skip_test "label" "reason"                      # record a skip
```

### Key tmux Details

- **Terminal size**: Default 120x40. Use `tmux new-session -d -s NAME -x WIDTH -y HEIGHT` for custom sizes.
- **Resize**: `tmux resize-window -t "$session" -x 80 -y 30` triggers `tea.WindowSizeMsg`.
- **Startup guard**: tinycode discards rune input for 2 seconds after first render. The 3-second sleep after `new_session` accounts for this.
- **Sidebar threshold**: Sidebar requires width >= 120 (defined in `internal/tui/layout.go`).
- **Leader key timeout**: 500ms — sleep 0.2 between `C-x` and the follow-up key.
- **Mouse**: Disabled via `TINYCODE_DISABLE_MOUSE=1` to prevent tmux mouse event interference.
- **Database**: Uses `TINYCODE_DB=:memory:` to avoid polluting the real session database.
- **Capture**: `tmux capture-pane -p` returns the visible terminal content as plain text (ANSI codes stripped).

## Plugin Functional Tests

**Script:** `script/test-plugins.sh`

Builds plugin binaries, sends JSON-RPC requests (initialize + tool/call) over stdin, and validates responses against live public APIs.

### Usage

```bash
./script/test-plugins.sh                    # all public API tests
./script/test-plugins.sh web-search         # single plugin group
./script/test-plugins.sh --list             # list test groups
```

### Environment Variables

| Variable | Default | Purpose |
|---|---|---|
| `PLUGIN_TEST_TIMEOUT` | `30` | Per-test timeout in seconds |

### Test Inventory

| Group | Plugin | Tests | External API |
|---|---|---|---|
| web-search | `plugin-web-search` | 3 | DuckDuckGo HTML (public, no auth) |
| rh-dev-content | `plugin-rh-dev-content` | 4 | developers.redhat.com (public) |
| rh-ecosystem-catalog | `plugin-rh-ecosystem-catalog` | 7 | Pyxis API at catalog.redhat.com (public) |

### How It Works

Each test:

1. Builds the plugin binary via `go build`
2. Sends a JSON-RPC `initialize` request followed by a `tool/call` request via stdin
3. Parses the second response line (tool call result)
4. Asserts the `content` field contains (or doesn't contain) expected text

### Writing New Plugin Tests

Use the built-in assertion helpers:

```bash
assert_tool_contains <plugin-name> <tool-name> '<json-args>' "label" "expected-text"
assert_tool_not_contains <plugin-name> <tool-name> '<json-args>' "label" "rejected-text"
skip_test "label" "reason"
```

For plugins requiring credentials, extend the script with environment-variable-gated test groups. See `docs/plugin-credentials.md` for each plugin's required options.

### Known Limitations

- **rh-dev-content RSS**: The `/blog/feed/` endpoint returns HTTP 403 from Akamai WAF. The test verifies graceful error handling instead.
- **web-search rate limiting**: DuckDuckGo may rate-limit frequent requests. Space test runs if you see "No results found" intermittently.

## CI Integration

All three suites exit non-zero on any failure and produce summary output suitable for CI logs.

```bash
# Full integration test run
make build
./script/test-headless.sh     # headless run-mode tests (requires Ollama)
./script/tui-test.sh          # TUI tests via tmux (core tests work without Ollama)
./script/test-plugins.sh      # plugin functional tests (requires internet)
```

For CI environments without Ollama, the headless suite will fail on prereq check (warns about missing Ollama). The TUI suite's core tests (T01-T26, T33) don't require a model, but T28-T30 will skip gracefully. Plugin tests require internet access but no credentials.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| All tests fail immediately | Binary not built | `make build` |
| Model tests fail | Ollama not running or model not pulled | `ollama serve` then `ollama pull qwen3.5:9b` |
| TUI tests fail with "tmux not found" | tmux not installed | `brew install tmux` |
| TC-H21 intermittent failure | Model slow to produce output within 60s | Increase `TINYCODE_TEST_TIMEOUT` or use a faster model |
| T29 always skips | Model doesn't support reasoning/thinking | Expected — use a reasoning model to exercise this test |
| TUI tests flaky | Timing sensitivity | Increase sleep values; check no other tmux sessions conflict |
| "tinycode binary not found" | Wrong path | Set `TINYCODE_BIN=path/to/binary` or `TINYCODE=path/to/binary` |
