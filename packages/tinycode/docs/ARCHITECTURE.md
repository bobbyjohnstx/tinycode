# Architecture: Tinycode

## Overview
Tinycode is a developer-centric AI orchestration framework designed to provide an intelligent, tool-augmented interface for codebase interaction. It acts as a bridge between large language models (LLMs) and the local development environment, enabling complex tasks like repository analysis, automated refactoring, and technical research through a highly structured orchestration layer.

The framework focuses on precision, reliability, and performance, treating the AI not just as a chatbot, but as an agent capable of manipulating a filesystem, executing shell commands, and interacting with language servers via a strictly defined capability registry.

## Core Architecture

### Layered Functional Architecture
Tinycode employs a layered architecture that separates the high-level orchestration logic from the low-level provider implementations. This ensures that the system can swap LLM providers or tool implementations without affecting the overall conversation flow or state management.

### Runtime and Concurrency
The framework is built on **Bun**, leveraging its high-performance runtime and native TypeScript support for rapid execution and efficient I/O.

### Dependency Injection and Logic (Effect v4 Beta)
Central to Tinycode's architecture is the **Effect** system (v4 beta). Effect provides a robust framework for:
- **Dependency Injection (DI)**: Using Layers to manage service lifecycles and dependencies.
- **Concurrency**: Managing asynchronous operations with fibers and scopes, ensuring predictable resource cleanup.
- **Error Handling**: Using tagged error classes for typed, recoverable failures.

### State Management
State is managed using a combination of global and instance-specific strategies:
- **`InstanceState`**: Used for per-directory or per-project state. This ensures that multiple open projects do not leak state into one another.
- **`ScopedCache`**: Keyed by directory, this mechanism ensures that resources for a specific instance are initialized once and automatically disposed of when the instance is closed.

## Technical Stack
- **Runtime**: TypeScript / Bun
- **Orchestration**: Effect (v4 beta)
- **Persistence**: SQLite with Drizzle ORM
- **AI Integration**: Vercel AI SDK
- **Extensibility**: MCP (Model Context Protocol) and a custom Tool Registry

## Key Modules

### `src/session`
Manages the conversation lifecycle. This includes the `SessionProcessor` which handles the turn-based loop between the user and the AI, message history management, and session compaction to handle context window limits.

### `src/tool`
The capabilities layer. It contains the `ToolRegistry` and individual tool implementations (e.g., `read`, `write`, `grep`, `shell`). Tools are defined as discrete units of functionality with strict schemas for input and output.

### `src/cli`
The entry point of the application, implementing the TUI (Terminal User Interface) and command-line arguments. It handles the raw input/output stream and routes events to the session processor.

### `src/provider`
Abstractions for LLM providers. It translates internal request formats into provider-specific API calls (e.g., Anthropic, OpenAI) and parses the responses back into the framework's internal message format.

### `src/permission`
A validation layer that evaluates whether a tool call is permitted based on the current context and security policies, preventing unauthorized filesystem or shell access.

## Data Flow
The system operates in a continuous loop:

1. **User Input**: User provides a prompt via the CLI/TUI.
2. **Session Processor**: The processor retrieves current session context and prompts.
3. **LLM Provider**: The request is sent to the chosen LLM via the provider abstraction.
4. **Tool Execution**: If the LLM requests a tool call, the `ToolRegistry` invokes the corresponding implementation.
5. **Persistence**: The result of the tool execution and the conversation state are persisted to the SQLite database.
6. **LLM Feedback**: The tool output is fed back to the LLM to generate the final response or a subsequent tool call.

## Project Structure
```text
packages/tinycode/
├── src/
│   ├── cli/                # TUI and CLI entry points
│   ├── control-plane/      # Workspace context and global state
│   ├── git/                # Git integration utilities
│   ├── permission/         # Tool call validation and security
│   ├── provider/           # LLM provider abstractions
│   ├── reference/          # Repository caching and indexing
│   ├── session/             # Conversation lifecycle & processing
│   │   ├── llm/            # Provider-specific request logic
│   │   └── prompt/         # System prompt templates
│   └── tool/               # Tool definitions and registry
│       └── shell/          # Shell execution environment
└── migration/              # Drizzle database migrations
```

## Key Patterns & Conventions

### Tool Registry
All system capabilities are registered in a central `ToolRegistry`. This allows the framework to dynamically generate the tool schemas presented to the LLM and ensures a consistent interface for tool execution and error handling.

### Effect-based Dependency Injection
Services are defined as Effect classes and provided through Layers. This allows for seamless testing (via mock layers) and precise control over the service lifecycle.

### Module Shape (Self-reexporting Namespaces)
To maintain ESM compatibility and enable tree-shaking, Tinycode avoids `export namespace`. Instead, it uses flat top-level exports with a self-reexport at the bottom of the file:

```ts
export class Service extends Context.Service<Service, Interface>()("@tinycode/Foo") {}
export const layer = Layer.effect(Service, ...)

export * as Foo from "./foo"
```
Consumers then import the namespace projection: `import { Foo } from "@/foo/foo"`.
