# Project Architecture Documentation

**Session ID:** ses_16c36f5c1ffeoO0K0QQS1rXmAX
**Created:** 6/4/2026, 12:58:06 PM
**Updated:** 6/4/2026, 1:02:28 PM

---

## User

@deep-explore this project and have @writer document the results in ARCHITECTURE.md

---

## Assistant (Build · gemma4:31b-cloud · 73.5s)

**Tool: task**

**Input:**
```json
{
  "prompt": "Perform a comprehensive exploration of the codebase to understand its architecture, core components, data flow, and design patterns. \n\nPlease identify:\n1. The overall project structure and the role of each main package/directory.\n2. Core architectural patterns used (e.g., Effect-TS, Drizzle ORM, TUI architecture).\n3. Key services, layers, and their interactions.\n4. Data models and how state is managed (e.g., InstanceState, ScopedCache).\n5. The build, test, and deployment pipeline.\n6. Any unique conventions or constraints defined in AGENTS.md or other config files.\n\nProvide a detailed technical summary of these findings. This summary will be used by a technical writer to create a comprehensive ARCHITECTURE.md file.",
  "subagent_type": "deep-explore"
}
```

**Output:**
```
<task id="ses_16c36c475ffeubGSnumTnvmKNJ" state="completed">
<task_result>
### Technical Architecture Summary: Tinycode (Opencode)

This document provides a comprehensive technical overview of the Tinycode codebase, designed for the creation of a formal `ARCHITECTURE.md`.

#### 1. Project Structure & Roles
The project is a Bun-based TypeScript monorepo (or package-centric structure) focusing on an AI-powered development environment.
- **`packages/tinycode/src/`**: Core application logic.
  - **`cli/`**: Command-line interface using `yargs`, managing top-level commands (MCP, TUI, Session, Agent, etc.).
  - **`project/`**: Project-level orchestration, including bootstrapping and VCS integration.
  - **`session/`**: Management of AI sessions, LLM integration, prompting, and message history.
  - **`effect/`**: Core infrastructure for dependency injection, runtime management, and state scoping.
  - **`storage/`**: Database abstraction layer (Drizzle ORM) and migration logic.
  - **`server/`**: HTTP API and backend services for external integration.
  - **`plugin/`**: Extensibility framework allowing the core to be mutated or extended.
  - **`lsp/`**: Language Server Protocol integration for IDE-like capabilities.
  - **`pty/`**: Pseudo-terminal handling for shell integration.

#### 2. Core Architectural Patterns
- **Effect-TS (v4 Beta)**: The primary engine for concurrency, dependency injection (Layers), and error handling. The codebase heavily utilizes `Effect.gen` for composition and `Layer` for service wiring.
- **Drizzle ORM**: Used for type-safe SQLite interactions. The system employs a "Migration-first" approach where schema changes are tracked in `.sql.ts` files and executed via `drizzle-kit`.
- **TUI (Text User Interface)**: A specialized interactive UI layer (referenced as `opentui`) that allows for live interaction with the AI agent.
- **Service-Oriented Architecture**: Logic is encapsulated in `Context.Service` classes, which are then provided via Layers.

#### 3. Key Services & Interactions
- **InstanceBootstrap**: Orchestrates the startup of all per-project services (Config, File, LSP, Plugin, VCS, etc.). It ensures that plugins are initialized before other services, as they may mutate global configurations.
- **Runtime Management**: 
  - `makeRuntime`: Creates a deduplicated runtime for services using a `memoMap`.
  - `EffectBridge`: Acts as a boundary between native asynchronous callbacks (e.g., file watchers, PTY) and the Effect runtime.
- **Bus**: A message-passing system used for inter-service communication.

#### 4. Data Models & State Management
- **Persistence**: SQLite is the primary data store. Database paths are determined by the `InstallationChannel` (prod, beta, local) or environment flags.
- **InstanceState**: A sophisticated state management pattern for multi-project environments.
  - **ScopedCache**: State is keyed by directory. When a project directory is opened, a corresponding state is initialized; when closed, it is invalidated.
  - **Lifecycle**: Uses `Effect.addFinalizer` to ensure cleanup of project-specific resources.
- **LocalContext**: Used for managing database transactions and effect-binding across synchronous call stacks.

#### 5. Pipeline: Build, Test, & Deploy
- **Runtime**: Powered by **Bun**, utilizing `bun run` and `bun test`.
- **Type Checking**: Managed via `tsgo --noEmit` (aliased as `typecheck` in `package.json`).
- **Database Workflow**: 
  - `bun run db generate` $\rightarrow$ creates SQL migrations.
  - `JsonMigration.run` $\rightarrow$ handles high-level data migrations during app startup.
- **Build**: A custom build script (`script/build.ts`) handles the packaging of the application.
- **Testing**: Unit tests are run via `bun test` with JUnit reporting for CI.

#### 6. Unique Conventions & Constraints
- **Module Shape**: Explicitly forbids `export namespace`. Instead, it uses a "Self-Reexport" pattern:
  ```ts
  export class Service ...
  export * as Foo from "./foo" // Projection for consumers
  ```
- **Effect Rules**: 
  - Mandatory use of `Effect.fn` for traced effects.
  - Preference for `DateTime.nowAsDate` and `FileSystem.FileSystem` over native Node.js APIs.
  - Use of branded schemas (`Schema.brand`) and tagged error classes.
- **Drizzle Naming**: Strict `snake_case` for table and column names to avoid mapping strings in the ORM definition.
- **Development**: TUI development is recommended via `tmux` to allow background execution of `bun dev` while inspecting output.
</task_result>
</task>
```

---

## Assistant (Build · gemma4:31b-cloud · 44.4s)

**Tool: task**

**Input:**
```json
{
  "prompt": "Convert the following technical architecture summary into a high-quality, professional `ARCHITECTURE.md` file. \n\nThe document should be structured for software engineers and should include the following sections:\n1. **Overview**: A high-level description of the project's purpose and primary goals.\n2. **Core Architectural Principles**: Detail the use of Effect-TS, Drizzle ORM, and the Project/Session/Service model.\n3. **Project Structure**: Explain the layout of the codebase and the responsibility of each major directory (CLI, Project, Session, Effect, Storage, Server, Plugin, LSP, PTY).\n4. **State & Data Management**: Document how the system handles persistence (SQLite), the `InstanceState` pattern, `ScopedCache`, and how multi-project state is isolated.\n5. **Component Interaction**: Describe how services are wired together via Layers and how the `InstanceBootstrap` orchestrates startup. Mention the `EffectBridge` for native boundaries.\n6. **Development Workflow**: Detail the build process (Bun), type-checking, database migration workflow, and TUI development (tmux strategy).\n7. **Coding Conventions**: Document the \"Self-Reexport\" module pattern, Effect-TS naming rules, and Drizzle schema conventions.\n\nInput Summary:\n---\n<task_result>\n### Technical Architecture Summary: Tinycode (Opencode)\n\nThis document provides a comprehensive technical overview of the Tinycode codebase, designed for the creation of a formal `ARCHITECTURE.md`.\n\n#### 1. Project Structure & Roles\nThe project is a Bun-based TypeScript monorepo (or package-centric structure) focusing on an AI-powered development environment.\n- **`packages/tinycode/src/`**: Core application logic.\n  - **`cli/`**: Command-line interface using `yargs`, managing top-level commands (MCP, TUI, Session, Agent, etc.).\n  - **`project/`**: Project-level orchestration, including bootstrapping and VCS integration.\n  - **`session/`**: Management of AI sessions, LLM integration, prompting, and message history.\n  - **`effect/`**: Core infrastructure for dependency injection, runtime management, and state scoping.\n  - **`storage/`**: Database abstraction layer (Drizzle ORM) and migration logic.\n  - **`server/`**: HTTP API and backend services for external integration.\n  - **`plugin/`**: Extensibility framework allowing the core to be mutated or extended.\n  - **`lsp/`**: Language Server Protocol integration for IDE-like capabilities.\n  - **`pty/`**: Pseudo-terminal handling for shell integration.\n\n#### 2. Core Architectural Patterns\n- **Effect-TS (v4 Beta)**: The primary engine for concurrency, dependency injection (Layers), and error handling. The codebase heavily utilizes `Effect.gen` for composition and `Layer` for service wiring.\n- **Drizzle ORM**: Used for type-safe SQLite interactions. The system employs a \"Migration-first\" approach where schema changes are tracked in `.sql.ts` files and executed via `drizzle-kit`.\n- **TUI (Text User Interface)**: A specialized interactive UI layer (referenced as `opentui`) that allows for live interaction with the AI agent.\n- **Service-Oriented Architecture**: Logic is encapsulated in `Context.Service` classes, which are then provided via Layers.\n\n#### 3. Key Services & Interactions\n- **InstanceBootstrap**: Orchestrates the startup of all per-project services (Config, File, LSP, Plugin, VCS, etc.). It ensures that plugins are initialized before other services, as they may mutate global configurations.\n- **Runtime Management**: \n  - `makeRuntime`: Creates a deduplicated runtime for services using a `memoMap`.\n  - `EffectBridge`: Acts as a boundary between native asynchronous callbacks (e.g., file watchers, PTY) and the Effect runtime.\n- **Bus**: A message-passing system used for inter-service communication.\n\n#### 4. Data Models & State Management\n- **Persistence**: SQLite is the primary data store. Database paths are determined by the `InstallationChannel` (prod, beta, local) or environment flags.\n- **InstanceState**: A sophisticated state management pattern for multi-project environments.\n  - **ScopedCache**: State is keyed by directory. When a project directory is opened, a corresponding state is initialized; when closed, it is invalidated.\n  - **Lifecycle**: Uses `Effect.addFinalizer` to ensure cleanup of project-specific resources.\n- **LocalContext**: Used for managing database transactions and effect-binding across synchronous call stacks.\n\n#### 5. Pipeline: Build, Test, & Deploy\n- **Runtime**: Powered by **Bun**, utilizing `bun run` and `bun test`.\n- **Type Checking**: Managed via `tsgo --noEmit` (aliased as `typecheck` in `package.json`).\n- **Database Workflow**: \n  - `bun run db generate` $\\rightarrow$ creates SQL migrations.\n  - `JsonMigration.run` $\\rightarrow$ handles high-level data migrations during app startup.\n- **Build**: A custom build script (`script/build.ts`) handles the packaging of the application.\n- **Testing**: Unit tests are run via `bun test` with JUnit reporting for CI.\n\n#### 6. Unique Conventions & Constraints\n- **Module Shape**: Explicitly forbids `export namespace`. Instead, it uses a \"Self-Reexport\" pattern:\n  ```ts\n  export class Service ...\n  export * as Foo from \"./foo\" // Projection for consumers\n  ```\n- **Effect Rules**: \n  - Mandatory use of `Effect.fn` for traced effects.\n  - Preference for `DateTime.nowAsDate` and `FileSystem.FileSystem` over native Node.js APIs.\n  - Use of branded schemas (`Schema.brand`) and tagged error classes.\n- **Drizzle Naming**: Strict `snake_case` for table and column names to avoid mapping strings in the ORM definition.\n- **Development**: TUI development is recommended via `tmux` to allow background execution of `bun dev` while inspecting output.\n</task_result>\n---\n\nPlease output only the content of the `ARCHITECTURE.md` file.",
  "subagent_type": "writer"
}
```

**Output:**
```
<task id="ses_16c359d8effeO7i08YMNgEWbcH" state="completed">
<task_result>
# Architecture: Tinycode (Opencode)

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
</task_result>
</task>
```

---

## Assistant (Build · gemma4:31b-cloud · 136.6s)

**Tool: write**

**Input:**
```json
{
  "content": "# Architecture: Tinycode (Opencode)\n\n## 1. Overview\nTinycode is an AI-powered development environment designed to integrate LLM capabilities directly into the software engineering workflow. The project provides a high-performance, extensible system featuring a Text User Interface (TUI), Language Server Protocol (LSP) integration, and a sophisticated plugin architecture. Its primary goals are to provide seamless AI-driven code manipulation, robust project orchestration, and a highly stable runtime for complex, concurrent operations.\n\n## 2. Core Architectural Principles\n\n### Effect-TS (v4 Beta)\nThe system is built on **Effect-TS**, serving as the primary engine for concurrency, dependency injection, and error handling. \n- **Composition**: Logic is composed using `Effect.gen` to manage asynchronous flows and dependencies linearly.\n- **Dependency Injection**: Services are defined as `Context.Service` classes and wired together using `Layer`. This allows for high testability and easy swapping of implementations.\n- **Error Handling**: The system utilizes tagged error classes and branded schemas (`Schema.brand`) to ensure type-safe error recovery and data validation.\n\n### Drizzle ORM\nPersistence is managed via **Drizzle ORM** targeting **SQLite**. \n- **Migration-First**: Schema changes are defined in `.sql.ts` files. Drizzle Kit is used to generate SQL migrations, ensuring the database schema is version-controlled and reproducible.\n- **Type Safety**: The ORM provides full TypeScript inference from the database schema to the application logic.\n\n### Project/Session/Service Model\nThe architecture follows a hierarchical ownership model:\n- **Project**: The top-level entity representing a codebase. It manages global configuration and project-specific service lifecycles.\n- **Session**: An instance of AI interaction within a project, maintaining message history, prompting context, and LLM state.\n- **Service**: Stateless or stateful logic providers (e.g., VCS, LSP, FileSystem) that are scoped to the project's runtime.\n\n## 3. Project Structure\nThe codebase is organized by domain responsibility to maintain a clear separation of concerns:\n\n| Directory | Responsibility |\n| :--- | :--- |\n| `cli/` | Command-line interface using `yargs` for top-level entry points (MCP, TUI, Session). |\n| `project/` | Orchestrates project bootstrapping, VCS integration, and initialization. |\n| `session/` | Manages LLM integration, prompt engineering, and conversation history. |\n| `effect/` | Core infrastructure for DI, runtime management, and state scoping. |\n| `storage/` | Database abstraction layer, Drizzle schema definitions, and migration logic. |\n| `server/` | HTTP API and backend services for external integrations. |\n| `plugin/` | Framework for extending or mutating core behavior via a plugin system. |\n| `lsp/` | Language Server Protocol implementation for IDE-like code intelligence. |\n| `pty/` | Pseudo-terminal handling for shell integration and command execution. |\n\n## 4. State & Data Management\n\n### Persistence\nThe system uses **SQLite** for all local persistence. Database paths are resolved based on the `InstallationChannel` (prod, beta, or local), ensuring environment isolation.\n\n### InstanceState & ScopedCache\nTo support multi-project environments, Tinycode employs the `InstanceState` pattern:\n- **ScopedCache**: Service state is keyed by the project directory. When a directory is opened, the corresponding state is initialized; when the directory is closed, the cache is invalidated.\n- **Isolation**: This ensures that configurations or cached data from one project do not leak into another.\n- **Lifecycle**: Resources are cleaned up using `Effect.addFinalizer`, preventing memory leaks of project-specific fibers or file handles.\n\n## 5. Component Interaction\n\n### Service Wiring & Bootstrapping\nServices are modularized into `Layer`s. The `InstanceBootstrap` class orchestrates the startup sequence:\n1. **Plugin Initialization**: Plugins are loaded first, as they may mutate global configurations or register new services.\n2. **Core Services**: Base services (Config, File System) are initialized.\n3. **Dependent Services**: Higher-level services (LSP, VCS) are started once their dependencies are satisfied.\n\n### Runtime Boundaries\n- **`makeRuntime`**: A utility that creates a deduplicated runtime for services using a `memoMap`, preventing redundant service instantiation.\n- **`EffectBridge`**: Since native APIs (e.g., `node-pty`, file watchers) rely on asynchronous callbacks, the `EffectBridge` is used to transition these external events back into the Effect runtime with the correct project context.\n- **Bus**: An internal message-passing system is used for decoupled inter-service communication.\n\n## 6. Development Workflow\n\n### Tooling & Build\n- **Runtime**: Powered by **Bun** for extremely fast execution and package management.\n- **Type Checking**: Performed via `bun typecheck` (running `tsgo --noEmit`) to ensure type integrity across the monorepo.\n- **Build Process**: Custom packaging is handled by `script/build.ts`.\n\n### Database Workflow\n1. **Schema Update**: Modify `.sql.ts` files.\n2. **Generation**: Run `bun run db generate` to produce SQL migration files.\n3. **Application**: `JsonMigration.run` handles high-level data migrations during the application startup sequence.\n\n### TUI Development\nBecause the TUI is an interactive foreground process, development is performed using **tmux**:\n- Run `bun dev` in a detached tmux session.\n- Use `tmux capture-pane` or attach to the session to inspect the live UI while maintaining a separate shell for editing and debugging.\n\n## 7. Coding Conventions\n\n### Self-Reexport Module Pattern\nTo optimize tree-shaking and maintain ESM compatibility, the codebase forbids `export namespace`. It uses a \"Self-Reexport\" pattern for module projections:\n```ts\n// Example: src/foo/foo.ts\nexport class Service extends Context.Service<Service, Interface>() {}\nexport const layer = Layer.effect(Service, ...)\n\nexport * as Foo from \"./foo\" // Consumers import { Foo } from \"@/foo/foo\"\n```\n\n### Effect-TS Naming & Rules\n- **Traced Effects**: `Effect.fn` must be used for all public-facing effects to ensure proper tracing.\n- **Platform APIs**: Prefer Effect-native wrappers (e.g., `FileSystem.FileSystem`, `DateTime.nowAsDate`) over raw Node.js/Web APIs.\n- **Schemas**: Use `Schema.brand` for single-value identifiers and `Schema.TaggedErrorClass` for domain errors.\n\n### Drizzle Conventions\n- **Naming**: All table and column names must use `snake_case`. This avoids the need to provide explicit string mappings in the ORM definition (e.g., `created_at: integer()`).\n",
  "filePath": "/Users/bjohns/projects/tinycode/packages/tinycode/ARCHITECTURE.md"
}
```

**Output:**
```
Wrote file successfully.
```

---

## Assistant (Build · gemma4:31b-cloud · 7.4s)

The project has been explored and the results have been documented in `ARCHITECTURE.md`.

---

