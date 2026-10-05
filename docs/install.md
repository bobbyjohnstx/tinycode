# Installation

tinycode is a single static binary with no runtime dependencies. Works with any OpenAI-compatible LLM endpoint --- local or cloud.

## Binary install (recommended)

### One-liner (`install.sh`)

Downloads the latest GitHub release for your platform and installs to `~/.local/bin`:

```bash
curl -fsSL https://raw.githubusercontent.com/bobbyjohnstx/tinycode/main/install.sh | sh
```

Supported platforms: macOS and Linux (`amd64` / `arm64`). Override defaults with environment variables:

| Variable | Default | Purpose |
|----------|---------|---------|
| `TINYCODE_INSTALL_DIR` | `$HOME/.local/bin` | Install destination |
| `VERSION` | latest release | Pin a release tag (for example `v2.1.3`) |
| `TINYCODE_REPO` | `bobbyjohnstx/tinycode` | GitHub `owner/repo` |
| `TINYCODE_BASE_URL` | `https://github.com` | Release download host |
| `TINYCODE_API_URL` | `https://api.github.com` | Releases API host |

If `~/.local/bin` is not on your `PATH`, the script prints shell-specific instructions.

### Homebrew

```bash
brew install bobbyjohnstx/tap/tinycode
```

### Manual download

Download a platform archive from [GitHub Releases](https://github.com/bobbyjohnstx/tinycode/releases) (for example `tinycode-darwin-arm64.tar.gz`), extract the `tinycode` binary, and place it on your `PATH`.

## Build from source

### Prerequisites

- **Go 1.27.1+** -- [download](https://go.dev/dl/) (matches `go.mod`)
- **make** -- included on macOS and most Linux distributions
- **git** -- to clone the repo

No C toolchain needed. SQLite is pure Go (`modernc.org/sqlite`), so CGO is not required.

### macOS

```bash
# Clone and build
git clone https://github.com/bobbyjohnstx/tinycode.git
cd tinycode
make build

# Run from the build directory
./dist/tinycode

# Or install to PATH
sudo cp dist/tinycode /usr/local/bin/
```

Go can be installed via Homebrew:

```bash
brew install go
```

### Linux

```bash
# Install Go 1.27.1+ from https://go.dev/dl/ (distro packages may be older)

# Clone and build
git clone https://github.com/bobbyjohnstx/tinycode.git
cd tinycode
make build

# Run
./dist/tinycode

# Or install to PATH
sudo cp dist/tinycode /usr/local/bin/
```

### Clipboard support (Linux)

For `/copy` and `/paste-image` to work on Linux, install a clipboard utility:

- **X11**: `sudo apt install xclip`
- **Wayland**: `sudo apt install wl-clipboard`

Without a clipboard utility, clipboard commands will fail with an error message. The rest of tinycode works fine.

### Image paste (Linux)

For `/paste-image` with image support, `xclip` must be compiled with image format support (the default package includes it). On Wayland, `wl-paste` handles images natively.

### Windows (WSL)

Native Windows is not supported -- bubbletea requires a Unix terminal.

Use WSL2 instead:

1. Install WSL2: `wsl --install` (from PowerShell as admin)
2. Open your WSL distribution (Ubuntu is the default)
3. Follow the Linux instructions above (binary install or build from source)

## Configuration

Config files are loaded with a 3-name fallback in each directory:

1. `tinycode.jsonc`
2. `tinycode.json`
3. `config.json`

Global config: `~/.config/tinycode/` (first matching name wins). Project config: `.tinycode/` walking up from the working directory (innermost wins). JSONC comments are supported.

```jsonc
// ~/.config/tinycode/tinycode.jsonc
{
  "model": "ollama/qwen3.5:9b",
  "default_agent": "build"
}
```

## Cross-compilation

Build for all supported platforms at once:

```bash
make build-all
```

This produces binaries in `dist/` for:

- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`

## Dependencies

| Requirement | When | Notes |
|-------------|------|-------|
| Go 1.27.1+ | Build only | Not needed at runtime; matches `go.mod` |
| make | Build only | For the Makefile targets |
| xclip / wl-clipboard | Runtime (optional) | Linux clipboard support |

The built binary is fully self-contained. No Node.js, no runtime dependencies, no separate server process. The web UI is embedded via `go:embed`.

## Verify installation

```bash
# Check version
tinycode version

# Run diagnostics (checks config, database, providers, agents, plugins, skills)
tinycode doctor

# Show CLI usage
tinycode help

# Show config and provider status
tinycode debug config
tinycode debug paths

# List discovered models (requires a running provider)
tinycode models
```

`tinycode doctor` is the recommended post-install check. It verifies every subsystem and exits non-zero if any critical check fails.

## Updating

### Binary install

Re-run `install.sh`, or `brew upgrade tinycode` if you installed via Homebrew.

### From source

Pull the latest source and rebuild:

```bash
cd tinycode
git pull
make build
```

If installed to PATH, copy the new binary:

```bash
sudo cp dist/tinycode /usr/local/bin/
```
