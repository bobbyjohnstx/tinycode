# Support

How to get help with tinycode.

## Installation

Build from source or download a pre-built binary from [GitHub Releases](https://github.com/bobbyjohnstx/tinycode/releases).

**From source:**

```bash
git clone https://github.com/bobbyjohnstx/tinycode.git
cd tinycode
make build        # produces dist/tinycode
```

**From releases:**

Download the binary for your platform from the releases page and place it on your `PATH`.

## Quick Diagnostics

Collect environment information for bug reports:

```bash
go version
./dist/tinycode --version
uname -a
```

## Getting Help

### 1. Check the Docs

- **[Getting Started](docs/getting-started.md)** -- Step-by-step walkthrough for first-time use
- **[Cheat Sheet](docs/cheatsheet.md)** -- Keyboard shortcuts and common commands
- **[Troubleshooting](docs/troubleshooting.md)** -- Solutions to common problems
- **[Architecture](docs/architecture.md)** -- How tinycode works internally
- **[CLAUDE.md](CLAUDE.md)** -- Development and configuration guidance

### 2. Search Issues and Discussions

- **[GitHub Issues](https://github.com/bobbyjohnstx/tinycode/issues)** -- Bug reports and feature requests
- **[GitHub Discussions](https://github.com/bobbyjohnstx/tinycode/discussions)** -- Questions and community chat

Chances are someone has hit your issue before.

### 3. Ask in Discussions

Open a [new discussion](https://github.com/bobbyjohnstx/tinycode/discussions/new):

1. **Title:** Brief description of your issue
2. **Category:** Choose from "Help", "Ideas", "General"
3. **Description:** Include:
   - What you are trying to do
   - What happened instead
   - Steps to reproduce
   - Output of `go version` and `./dist/tinycode --version`
   - Your config (`~/.config/tinycode/config.json`)

Community members and maintainers will help.

## Common Issues

### Provider not connecting (Ollama)

Ollama must be running before tinycode can discover models. Start it with:

```bash
ollama serve
```

Verify it is reachable:

```bash
curl http://localhost:11434/api/tags
```

If using a non-default Ollama host, set it in your config file (`~/.config/tinycode/config.json`).

### Model not found

tinycode discovers models from configured providers on startup. If a model is missing:

1. Confirm the model is pulled locally (`ollama list`) or available from your provider
2. Restart tinycode to trigger re-discovery
3. Check your config for `enabled_providers` or `disabled_providers` filters

### Config file format errors

tinycode supports JSONC (JSON with comments). Common issues:

- Trailing commas after the last item in an object or array
- Unquoted keys
- Invalid JSON syntax

Validate your config with:

```bash
cat ~/.config/tinycode/config.json | python3 -c "import sys,json; json.load(sys.stdin); print('valid')"
```

Note: This does not validate JSONC comments -- strip `//` lines before checking if needed.

## Reporting Bugs

File a [bug report](https://github.com/bobbyjohnstx/tinycode/issues/new) if:

- tinycode crashes
- A feature does not work as documented
- You find a security vulnerability (see Security section below)

**Include:**
- Steps to reproduce
- Expected behavior
- Actual behavior
- Environment: OS, Go version, tinycode version, LLM provider and model
- Relevant config and logs

**Good bug report example:**

```
Title: "Session won't load after export"

Steps to reproduce:
1. Create a session with 50+ messages
2. Export with <leader>x
3. Restart tinycode
4. List sessions with <leader>l

Expected: Session appears in list
Actual: Session missing, error in logs

Environment:
- macOS 14.3
- Go 1.23.1
- tinycode v2.0.0
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

- Ensure Go is installed: `go version`
- If Ollama is slow, check Activity Monitor for CPU/memory usage
- TTY issues? Try `export TERM=xterm-256color` before running tinycode

### Windows

- Use Windows Terminal (not cmd.exe) for better compatibility
- If terminal rendering looks wrong, try a different font (Cascadia Code works well)

### Linux

- If terminal does not render colors, check `TERM` variable
- On headless servers, use `./dist/tinycode serve` for the headless API mode
- SELinux: May need to adjust contexts for file access

## LLM Model Help

For model recommendations, compatibility notes, and provider-specific guidance, see [Model Compatibility](docs/model-compatibility.md).

### Slow Responses?

1. Check system resources: `top`, `nvidia-smi` (if GPU available)
2. Try a smaller model for faster inference
3. Use a quantized version (Q4 instead of Q5)
4. See [Troubleshooting](docs/troubleshooting.md)

### Model Not Detected?

Verify your provider is running and accessible. See [Troubleshooting](docs/troubleshooting.md) and [Model Compatibility](docs/model-compatibility.md).

## Remote Deployment Help

Deploying tinycode on a remote server or in Kubernetes?

- **Remote server:** See [Deployment Guide](docs/deployment.md)
- **Container:** See [Deployment Guide](docs/deployment.md#container-deployment)
- **Kubernetes:** See [tinycode-operator](https://github.com/bobbyjohnstx/tinycode-operator)

For cluster-specific issues, check the [operator's troubleshooting guide](https://github.com/bobbyjohnstx/tinycode-operator#troubleshooting).

## Learning More

- **[Architecture deep-dive](docs/architecture.md)** -- How the system works
- **[User Guide](docs/user-guide.md)** -- Detailed usage instructions
- **[CLAUDE.md](CLAUDE.md)** -- Developer guide and project structure
- **[AGENTS.md](AGENTS.md)** -- Coding style and conventions

## Contact

- **GitHub:** [@bobbyjohnstx/tinycode](https://github.com/bobbyjohnstx/tinycode)
- **Issues:** [GitHub Issues](https://github.com/bobbyjohnstx/tinycode/issues)
- **Discussions:** [GitHub Discussions](https://github.com/bobbyjohnstx/tinycode/discussions)
- **Security:** See [SECURITY.md](SECURITY.md)
