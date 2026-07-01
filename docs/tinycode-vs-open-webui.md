# tinycode + Open WebUI: Using Both

tinycode and Open WebUI are complementary tools that connect to the same local LLMs. They solve different problems.

## What each does

**Open WebUI** is a chat interface — a self-hosted ChatGPT. You ask questions, paste context, get answers. It handles conversations, document uploads, image generation, and multi-user access. It's great for general-purpose LLM interaction.

**tinycode** is a coding agent. It doesn't just answer questions about code — it reads your files, runs commands, edits code, and executes multi-step tasks autonomously. You describe what you want, and it works through the problem: reading files, running tests, making changes, verifying results.

## When to use which

| Task | Use | Why |
|------|-----|-----|
| "Explain how Kubernetes networking works" | Open WebUI | General knowledge question — no files to read |
| "Fix the failing test in src/auth.ts" | tinycode | Needs to read the file, understand the error, edit the code, run the test |
| "Summarize this PDF" | Open WebUI | Document upload and RAG pipeline |
| "Refactor the database layer to use connection pooling" | tinycode | Multi-file code changes with verification |
| "Draft an email to the team about the release" | Open WebUI | Writing task — no code tools needed |
| "Add input validation to all API endpoints" | tinycode | Needs to find endpoints, read code, edit files, run linter |
| "Compare React vs Svelte for our use case" | Open WebUI | Research and analysis conversation |
| "Review the PR on branch feature/auth" | tinycode | Needs git access, file reading, code analysis |
| "Help me brainstorm project names" | Open WebUI | Creative conversation |
| "Set up CI/CD with GitHub Actions" | tinycode | Creates workflow files, tests them, iterates |

## How they share infrastructure

Both can point at the same Ollama instance. No duplication needed.

```
┌─────────────┐     ┌──────────────────┐
│  Open WebUI │────>│                  │
│  (chat UI)  │     │  Ollama / vLLM   │
└─────────────┘     │  (local LLMs)    │
                    │                  │
┌─────────────┐     │  llama3.2        │
│  tinycode   │────>│  qwen3.5:9b      │
│  (code agent)     │  codestral       │
└─────────────┘     └──────────────────┘
```

- **Ollama** runs on `localhost:11434` — both tools auto-discover it
- **vLLM** runs on `localhost:8000` — both can connect
- Models are downloaded once, shared by both tools
- They can run simultaneously — Ollama handles concurrent requests

## Setup

If you already have Open WebUI + Ollama running:

```bash
# tinycode auto-discovers your Ollama instance — just start it
bun dev

# Or point at a specific Ollama host
TINYCODE_OLLAMA_HOST=http://192.168.1.100:11434 bun dev
```

No additional model downloads needed. tinycode uses the same models you already pulled for Open WebUI.

## The key difference

Open WebUI is a **conversation** with an LLM. You provide context, it responds.

tinycode is a **collaboration** with an LLM. It has tools — file access, shell execution, code editing, LSP integration — and uses them autonomously to accomplish tasks. The LLM decides which tools to call, inspects the results, and continues until the job is done.

Think of it this way: Open WebUI is asking an expert for advice. tinycode is hiring an expert to do the work.
