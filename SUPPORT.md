# Support

How to get help with tinycode.

## Installation

```bash
# Pick one:
curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/dev/install.sh | sh
npx tinycode-ai                          # or: npm install -g tinycode-ai
brew install bobbyjohnstx/tap/tinycode   # macOS / Linux
```

## Quick Diagnostics

Run this command to check your setup:

```bash
/tc-doctor
```

This diagnoses configuration, connectivity, permissions, and more. It's the first step for most issues.

## Getting Help

### 1. Check the Docs

- **[Getting Started](docs/getting-started.md)** — Step-by-step walkthrough for first-time use
- **[Cheat Sheet](docs/cheatsheet.md)** — Keyboard shortcuts and common commands
- **[Troubleshooting](docs/troubleshooting.md)** — Solutions to common problems
- **[Architecture](docs/architecture.md)** — How tinycode works internally
- **[CLAUDE.md](CLAUDE.md)** — Development and configuration guidance

### 2. Search Issues & Discussions

- **[GitHub Issues](https://github.com/bobbyjohnstx/tinycode/issues)** — Bug reports and feature requests
- **[GitHub Discussions](https://github.com/bobbyjohnstx/tinycode/discussions)** — Questions and community chat

Chances are someone's hit your issue before.

### 3. Ask in Discussions

Open a [new discussion](https://github.com/bobbyjohnstx/tinycode/discussions/new):

1. **Title:** Brief description of your issue
2. **Category:** Choose from "Help", "Ideas", "General"
3. **Description:** Include:
   - What you're trying to do
   - What happened instead
   - Steps to reproduce
   - Output of `/tc-doctor`
   - Your config (`~/.config/tinycode/config.json`)

Community members and maintainers will help.

## Reporting Bugs

File a [bug report](https://github.com/bobbyjohnstx/tinycode/issues/new) if:

- tinycode crashes
- A feature doesn't work as documented
- You find a security vulnerability (see Security section below)

**Include:**
- Steps to reproduce
- Expected behavior
- Actual behavior
- Output of `/tc-doctor`
- Relevant config and logs
- Your environment (OS, Bun version, LLM model)

**Good bug report example:**

```
Title: "Session won't load after export"

Steps to reproduce:
1. Create a session with 50+ messages
2. Export with `<leader>x`
3. Restart tinycode
4. List sessions with `<leader>l`

Expected: Session appears in list
Actual: Session missing, error in logs

Environment:
- macOS 14.3
- Bun 1.1.5
- tinycode v0.1.7
```

## Feature Requests

Have an idea? [Start a discussion](https://github.com/bobbyjohnstx/tinycode/discussions/new) in the "Ideas" category.

Describe:
- What you want to do
- Why it would help
- How you envision it working

Maintainers will evaluate and may create a tracked issue if it aligns with the roadmap.

See [Roadmap](docs/roadmap.md) for planned features.

## Security Issues

Found a security vulnerability? **Do not open a public issue.**

Email security concerns to the maintainers (see SECURITY.md), or use GitHub's [Report a security vulnerability](https://github.com/bobbyjohnstx/tinycode/security/advisories/new) feature.

Details in [SECURITY.md](SECURITY.md).

## Contributing Code

Want to fix a bug or add a feature?

1. **Fork** the repository
2. **Create a branch:** `git checkout -b fix/your-issue-name`
3. **Make changes** and test locally
4. **Write tests** for new functionality
5. **Commit:** Follow [conventional commits](https://www.conventionalcommits.org/)
6. **Push** and open a PR with a clear description

See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines and development setup.

## Platform-Specific Help

### macOS

- Ensure Bun is installed: `bun --version`
- If Ollama is slow, check Activity Monitor for CPU/memory usage
- TTY issues? Try `export TERM=xterm-256color` before `bun dev`

### Windows

- Use Windows Terminal (not cmd.exe) for better compatibility
- Ensure Bun for Windows is installed: `bun --version`
- If terminal rendering looks wrong, try a different font (Cascadia Code works well)
- Firewall: Open port 4096 if running headless server

### Linux

- Ensure Bun is installed: `bun --version`
- If terminal doesn't render colors, check `TERM` variable
- On headless servers, use `bun dev serve --hostname 0.0.0.0` to bind all interfaces
- SELinux: May need to adjust contexts for file access

## Remote Deployment Help

Deploying tinycode on a remote server or in Kubernetes?

- **Remote server:** See [Getting Started](docs/getting-started.md#step-3-run-tinycode) and [Troubleshooting](docs/troubleshooting.md#server--network-issues)
- **Container:** See [Container images](README.md#container-images)
- **Kubernetes:** See [tinycode-operator](https://github.com/bobbyjohnstx/tinycode-operator)

For cluster-specific issues, check the [operator's troubleshooting guide](https://github.com/bobbyjohnstx/tinycode-operator#troubleshooting).

## LLM Model Help

### Model Recommendations

- **Coding:** mistral, neural-chat, codellama
- **General:** llama2, llama3, solar
- **Multilingual:** qwen, starchat
- **Small (< 4B):** phi, tinyllama (slower but runs anywhere)

See [OpenRouter](https://openrouter.ai) for a full list and benchmarks.

### Slow Responses?

1. Check system resources: `top`, `nvidia-smi` (if GPU available)
2. Try a smaller model for faster inference
3. Use a quantized version (Q4 instead of Q5)
4. See [Troubleshooting](docs/troubleshooting.md#model-responses-are-very-slow)

### Model Not Detected?

Run `/tc-doctor` and check the provider status. See [Troubleshooting](docs/troubleshooting.md#llm-connection-issues).

## Slack / Chat

No dedicated Slack, but:

- **GitHub Discussions** is the main community forum
- **Issues** are used for tracked bugs and features
- Follow the project on GitHub for updates

## Sponsorship

tinycode is open source and free. If you find it useful:

- Star the repo on GitHub
- Share it with your team
- Contribute code or documentation
- Report bugs and suggest improvements

## Learning More

- **[Architecture deep-dive](docs/architecture.md)** — How the system works
- **[Agent prompt tiers](docs/agent-prompt-tiers.md)** — Small model optimization
- **[CLAUDE.md](CLAUDE.md)** — Developer guide and project structure
- **[AGENTS.md](AGENTS.md)** — Coding style and conventions

## Contact

- **GitHub:** [@bobbyjohnstx/tinycode](https://github.com/bobbyjohnstx/tinycode)
- **Issues:** [GitHub Issues](https://github.com/bobbyjohnstx/tinycode/issues)
- **Discussions:** [GitHub Discussions](https://github.com/bobbyjohnstx/tinycode/discussions)
- **Security:** See [SECURITY.md](SECURITY.md)
