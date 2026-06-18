# Architecture: Tinycode (tinycode)

## 1. Overview
Tinycode is an AI-powered development environment designed to integrate LLM capabilities directly into the software engineering workflow. The project provides a high-performance, extensible system featuring a Text User Interface (TUI), Language Server Protocol (LSP) integration, and a sophisticated plugin architecture. Its primary goals are to provide seamless AI-driven code manipulation, robust project orchestration, and a highly stable runtime for complex, concurrent operations.

## 2. Core Architectural Principles

### Effect-TS (v4 Beta)
The system is built on **Effect-TS**, serving as the primary engine for concurrency, dependency injection, and error handling. 
- **Composition**: Logic is composed using `Effect.gen` to manage asynchronous flows and dependencies linearly.
- **Dependency Injection**: Services are defined as `Context.Service` classes and wired together using `Layer`. This allows for high testability and easy swapping of implementations.
- **Error Handling**: The system utilizes tagged error classes and branded schemas (`Schema.brand`) to ensure type-safe error recovery and data validation.

### Drizzle ORM
Persistence is managed via **Drizzle ORM** targeting **SQLite**. 
- **Migration-First**: Schema changes are defined in `.sql.ts` files. Drizzle Kit is used to generate SQL migrations, ensuring the database schema is version-controlled and reproducible.
- **Type Safety**: The ORM provides full TypeScript inference from the database schema to the application logic.

### Project/Session/Service Model
The architecture follows a hierarchical ownership model:
- **Project**: The top-level entity representing a codebase. It manages global configuration and project-specific service lifecycles.
- **Session**: An instance of AI interaction within a project, maintaining message history, prompting context, and LLM state.
- **Service**: Stateless or stateful logic providers (e.g., VCS, LSP, FileSystem) that are scoped to the project's runtime.

## 3. Project Structure
The codebase is organized by domain responsibility to maintain a clear separation of concerns:

| Directory | Responsibility |
| :--- | :--- |
| `cli/` | Command-line interface using `yargs` for top-level entry points (MCP, TUI, Session). |
| `project/` | Orchestrates project bootstrapping, VCS integration, and initialization. |
| `session/` | Manages LLM integration, prompt engineering, and conversation history. |
| `effect/` | Core infrastructure for DI, runtime management, and state scoping. |
| `storage/` | Database abstraction layer, Drizzle schema definitions, and migration logic. |
| `server/` | HTTP API and backend services for external integrations. |
| `plugin/` | Framework for extending or mutating core behavior via a plugin system. |
| `lsp/` | Language Server Protocol implementation for IDE-like code intelligence. |
| `pty/` | Pseudo-terminal handling for shell integration and command execution. |

## 4. State & Data Management

### Persistence
The system uses **SQLite** for all local persistence. Database paths are resolved based on the `InstallationChannel` (prod, beta, or local), ensuring environment isolation.

### InstanceState & ScopedCache
To support multi-project environments, Tinycode employs the `InstanceState` pattern:
- **ScopedCache**: Service state is keyed by the project directory. When a directory is opened, the corresponding state is initialized; when the directory is closed, the cache is invalidated.
- **Isolation**: This ensures that configurations or cached data from one project do not leak into another.
- **Lifecycle**: Resources are cleaned up using `Effect.addFinalizer`, preventing memory leaks of project-specific fibers or file handles.

## 5. Component Interaction

### Service Wiring & Bootstrapping
Services are modularized into `Layer`s. The `InstanceBootstrap` class orchestrates the startup sequence:
1. **Plugin Initialization**: Plugins are loaded first, as they may mutate global configurations or register new services.
2. **Core Services**: Base services (Config, File System) are initialized.
3. **Dependent Services**: Higher-level services (LSP, VCS) are started once their dependencies are satisfied.

### Runtime Boundaries
- **`makeRuntime`**: A utility that creates a deduplicated runtime for services using a `memoMap`, preventing redundant service instantiation.
- **`EffectBridge`**: Since native APIs (e.g., `node-pty`, file watchers) rely on asynchronous callbacks, the `EffectBridge` is used to transition these external events back into the Effect runtime with the correct project context.
- **Bus**: An internal message-passing system is used for decoupled inter-service communication.

## 6. Development Workflow

### Tooling & Build
- **Runtime**: Powered by **Bun** for extremely fast execution and package management.
- **Type Checking**: Performed via `bun typecheck` (running `tsgo --noEmit`) to ensure type integrity across the monorepo.
- **Build Process**: Custom packaging is handled by `script/build.ts`.

### Database Workflow
1. **Schema Update**: Modify `.sql.ts` files.
2. **Generation**: Run `bun run db generate` to produce SQL migration files.
3. **Application**: `JsonMigration.run` handles high-level data migrations during the application startup sequence.

### TUI Development
Because the TUI is an interactive foreground process, development is performed using **tmux**:
- Run `bun dev` in a detached tmux session.
- Use `tmux capture-pane` or attach to the session to inspect the live UI while maintaining a separate shell for editing and debugging.

## 7. Coding Conventions

### Self-Reexport Module Pattern
To optimize tree-shaking and maintain ESM compatibility, the codebase forbids `export namespace`. It uses a "Self-Reexport" pattern for module projections:
```ts
// Example: src/foo/foo.ts
export class Service extends Context.Service<Service, Interface>() {}
export const layer = Layer.effect(Service, ...)

export * as Foo from "./foo" // Consumers import { Foo } from "@/foo/foo"
```

### Effect-TS Naming & Rules
- **Traced Effects**: `Effect.fn` must be used for all public-facing effects to ensure proper tracing.
- **Platform APIs**: Prefer Effect-native wrappers (e.g., `FileSystem.FileSystem`, `DateTime.nowAsDate`) over raw Node.js/Web APIs.
- **Schemas**: Use `Schema.brand` for single-value identifiers and `Schema.TaggedErrorClass` for domain errors.

### Drizzle Conventions
- **Naming**: All table and column names must use `snake_case`. This avoids the need to provide explicit string mappings in the ORM definition (e.g., `created_at: integer()`).
