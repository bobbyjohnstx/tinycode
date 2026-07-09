# Getting Started with tinycode

Step-by-step walkthrough to get tinycode running and productive in 15 minutes.

## Prerequisites

- **Bun** 1.1+ ([install here](https://bun.sh))
- **Git**
- One of:
  - **Ollama** (local LLM inference) — [install here](https://ollama.ai)
  - **vLLM** (faster inference) — `pip install vllm`
  - **API key** to OpenRouter, Anthropic, OpenAI, or Google

## Step 1: Install tinycode

### Quick install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/dev/install.sh | sh
```

This downloads the latest binary for your platform and installs to `~/.local/bin`.

### Alternative install methods

```bash
# npm (requires Node.js)
npx tinycode-ai

# Homebrew (macOS/Linux)
brew install bobbyjohnstx/tap/tinycode

# From source (for development)
git clone https://github.com/bobbyjohnstx/tinycode.git
cd tinycode
bun install
bun dev
```

## Step 2: Start an LLM (Local Option)

If you want to run **locally** (no cloud), start Ollama:

```bash
# In a separate terminal
ollama serve

# In another terminal, pull a model (3-5 min)
ollama pull qwen3:14b  # recommended — excellent tool calling
# or qwen3.5:9b for faster responses with good tool support
# or mistral, neural-chat, etc. (may struggle with tool calling if <7B)
```

**Note on model selection:** tinycode works best with models that support tool calling. Larger models (≥9B) have better tool-call accuracy. Models <7B may not support tool calling at all — tinycode detects this and works without tools in that case.

**Skip this step** if you're using OpenRouter, Anthropic, or another cloud provider.

## Step 3: Run tinycode

```bash
# If installed via curl/npm/brew
tinycode

# If running from source
bun dev
```

This starts tinycode in **TUI mode** against your current directory. You'll see:
- Session sidebar on the left
- Conversation area in the center
- Input prompt at the bottom

**What happened:**
- Detected Ollama running at `localhost:11434`
- Auto-discovered available models
- Loaded default build agent (full tool access)

## Step 4: Select a Model

Press `<leader>m` (Ctrl+X, then M):

```
List Models
────────────
ollama/qwen3:14b
ollama/mistral
```

Select one with arrow keys, press Enter. The model is ready.

## Step 5: Your First Conversation

Type a simple prompt:

```
Explain what this repository does in 2 sentences.
```

Press Enter. tinycode will:
1. Read files in the current directory
2. Ask the LLM to analyze them
3. Stream the response

Try more prompts:

```
What files are in src/?
Write a bash script that lists all .ts files
```

## Step 6: Using Agents

Agents are specialized personas that solve specific problems. Press `Tab` to cycle:

- **architect** — reads code, suggests designs (read-only)
- **code-reviewer** — severity-rated code review
- **debugger** — root-cause analysis
- **executor** — focused task implementation
- **planner** — break down complex work
- **test-engineer** — write tests

Try it:

```
Tab                              # Switch to code-reviewer
Now look at src/index.ts and review it for style
```

Or invoke directly:

```
/ask planner help me refactor this component
/ask test-engineer write unit tests for auth
```

## Step 7: Using Skills (Slash Commands)

Skills inject specialized instructions. Type `/` to see available ones:

```
/debug          # Isolate a single most-likely root cause
/trace          # Evidence-driven causal tracing with hypotheses
/verify         # Confirm changes actually work
/tc-doctor      # Diagnose tinycode configuration
/deepinit       # Generate codebase documentation structure
```

Example:

```
My tests are failing. Why?
/debug
```

This tells the debugger to focus on root-cause analysis, not fix everything.

## Step 8: Work with Your Code

tinycode has full access to read, write, and edit files. Reference files:

```
Edit @src/auth.ts to add a login function
```

Or use existing tools:

```
Show me the structure of packages/cli/
Find all TODO comments in TypeScript files
```

Use `<leader>e` to open your editor if you prefer:

```
<leader>e       # Opens your configured editor
# Edit, save, return to tinycode
```

## Step 9: Session Management

Sessions are conversations. Press `<leader>l` to see all:

```
All Sessions
────────────
My first session   (active)
Code review        (pinned)
Architecture notes
```

**Create a new session:** `<leader>n`

**Session hierarchy:** Sessions form a tree. Child sessions inherit context from their parent. Press `<leader>j` (go to first child) or `<leader>k` (go to parent).

**Pin a session:** `Ctrl+F` — keeps it visible when switching

## Step 10: Access the Web UI

The TUI is fast, but you can also use tinycode in a browser:

```bash
# In a separate terminal (TUI still running)
bun run --cwd packages/app dev

# Or start server + web in one command
bun dev web
```

Open `http://localhost:4096`. Same features as TUI: agents, skills, sessions, model switching.

## Configuration

Create `~/.config/tinycode/config.json`:

```json
{
  "model": "ollama/qwen3:14b",
  "lsp": true
}
```

**Useful settings:**

```json
{
  "model": "ollama/mistral",           // default model
  "lsp": true,                         // enable code intelligence
  "animations": false,                 // disable if TUI is slow
  "server": {
    "port": 4096,
    "hostname": "127.0.0.1"
  },
  "keybinds": {
    "leader": "space"                  // change leader key to space
  }
}
```

## Next Steps

### 1. Set up a cloud provider (optional)

For higher quality responses, use Anthropic, OpenAI, or OpenRouter:

```bash
export OPENROUTER_API_KEY=your-key
```

Then `<leader>m` and select an OpenRouter model.

### 2. Connect MCP servers

MCP servers add tools (web search, code execution, etc.):

```
<leader>a          # Switch to an agent
/mcp-setup         # Configure MCP servers
```

### 3. Explore agents in depth

Each agent has unique strengths. Try:

```
/ask architect analyze the data flow in auth.ts
/ask scientist what's the best testing strategy for this module?
/ask designer redesign the onboarding screen
```

### 4. Build a custom workflow

Create a `.tinycode/plans/` directory to keep work structured:

```
<leader>n          # New session
/ask planner help me plan the refactor
(planner writes to .tinycode/plans/my-plan.md)
/ask executor implement the first step of the plan
```

### 5. Explore the documentation

- [Architecture overview](architecture.md) — how tinycode works
- [Cheat sheet](cheatsheet.md) — quick keyboard shortcuts
- [Troubleshooting](troubleshooting.md) — common issues and fixes
- [Use cases](use-cases.md) — deployment patterns

## Common Patterns

### Code review workflow
```
Tab                                    # Switch to code-reviewer
Here's my PR: @src/auth.ts
(reviewer gives feedback)
Tab                                    # Switch to code-simplifier
Apply those suggestions
```

### Build a feature
```
<leader>n                              # New session
/ask planner break down building a login form
<leader>n                              # Child session for implementation
/ask executor implement the UI based on the plan
<leader>n                              # Another child for tests
/ask test-engineer write tests for login
```

### Debug a failing test
```
/ask debugger why is src/index.test.ts failing?
/ask executor fix the issue
```

### Document your code
```
/ask writer document the session processor architecture
```

## Keyboard Shortcuts

**Most important:**

| Key | Purpose |
|-----|---------|
| `Ctrl+X` (leader) | Prefix for many commands |
| `Tab` | Next agent |
| `<leader>a` | List agents |
| `<leader>l` | List sessions |
| `<leader>n` | New session |
| `<leader>m` | Switch model |
| `<leader>b` | Toggle sidebar |
| `Escape` | Stop current operation |
| `F1` | Help |

See [cheatsheet.md](cheatsheet.md) for complete keyboard reference.

## Troubleshooting

**Model not found:** Run `/tc-doctor` — it diagnoses configuration issues.

**Can't connect to Ollama:** Make sure `ollama serve` is running and listening on `localhost:11434`.

**Session won't load:** Check disk space and database health:
```bash
df -h
sqlite3 ~/.config/tinycode/db.sqlite ".tables"
```

See [troubleshooting.md](troubleshooting.md) for detailed solutions.

## Tips & Tricks

- Use `@filename` to reference files in your prompt
- `<leader>h` toggles code block collapse — focus on text, hide code
- `<leader>c` compacts a session, removing noise
- `<leader>x` exports to shareable HTML or JSON
- Sessions form a tree — organize by topic with parent/child relationships
- Use `/ask <agent>` inline instead of switching agents
- Press `?` in diff viewer for extra navigation shortcuts

## What's Next?

You're ready to start coding with AI! Explore:

1. **More agents** — try `architect`, `debugger`, `test-engineer`
2. **Skills** — `/tc-doctor`, `/trace`, `/debug` are powerful
3. **Web UI** — `bun dev web` for browser access
4. **Desktop app** — `bun run --cwd packages/desktop dev` for native app
5. **Remote deployment** — See [use-cases.md](use-cases.md) for server setup

Questions? Check [troubleshooting.md](troubleshooting.md) or file an issue on [GitHub](https://github.com/bobbyjohnstx/tinycode).
