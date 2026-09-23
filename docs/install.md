# Installation

tinycode is a single static binary with no runtime dependencies. Build from source, then run.

## Prerequisites

- **Go 1.22+** -- [download](https://go.dev/dl/)
- **make** -- included on macOS and most Linux distributions
- **git** -- to clone the repo

No C toolchain needed. SQLite is pure Go (`modernc.org/sqlite`), so CGO is not required.

## macOS

```bash
# Clone and build
git clone https://github.com/bobbyjohnstx/tinycode-go.git
cd tinycode-go
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

## Linux

```bash
# Install Go (Debian/Ubuntu)
sudo apt install golang

# Or download from https://go.dev/dl/ for the latest version

# Clone and build
git clone https://github.com/bobbyjohnstx/tinycode-go.git
cd tinycode-go
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

## Windows (WSL)

Native Windows is not supported -- bubbletea requires a Unix terminal.

Use WSL2 instead:

1. Install WSL2: `wsl --install` (from PowerShell as admin)
2. Open your WSL distribution (Ubuntu is the default)
3. Follow the Linux instructions above

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
| Go 1.22+ | Build only | Not needed at runtime |
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

Pull the latest source and rebuild:

```bash
cd tinycode-go
git pull
make build
```

If installed to PATH, copy the new binary:

```bash
sudo cp dist/tinycode /usr/local/bin/
```
