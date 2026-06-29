# tinycode Desktop

The tinycode Desktop app, built with Electron. Wraps the web UI in a native window with system tray integration, auto-updates, and platform-specific features for macOS, Windows, and Linux.

## Features

- **System tray integration**: Access tinycode from the menu bar (macOS) or system tray (Windows/Linux)
- **Global hotkey** (Cmd/Ctrl+Shift+T): Bring the window to front or minimize
- **Auto-updates**: Checks GitHub Releases for updates and notifies via in-app banner
- **Cross-platform menus**: Application menus with Help links to GitHub (repo, discussions, issues)
- **Theme sync**: Automatically detects OS dark/light mode changes
- **Persistent settings**: Zoom level and window state saved across restarts
- **Security**: Content Security Policy headers, navigation origin validation, URL scheme validation on external links
- **Platform-specific behavior**:
  - macOS: Closing the window keeps the app running in the dock. Cmd+Q quits fully. Dock badge shows notification count.
  - Windows/Linux: Taskbar flashes on background notifications

## Development

From the repo root:

```bash
# Install dependencies
bun install

# Launch Electron app in development
bun run --cwd packages/desktop dev
```

## Build

From the repo root:

```bash
# Build web UI assets and package as distributable app
bun run --cwd packages/desktop build && bun run --cwd packages/desktop package
```

The resulting app will be in `packages/desktop/out/` with platform-specific installers and binaries ready for distribution.
