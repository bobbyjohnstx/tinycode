# Plugin Development Guide

This guide covers everything you need to build, test, and publish plugins for tinycode.

## Overview

Plugins extend tinycode with custom tools, LLM providers, authentication flows, and lifecycle hooks. A plugin is an npm package (or local file) that exports a `PluginModule` object. When tinycode loads the plugin, it calls the module's `server` function with a `PluginInput` context and receives back a `Hooks` object declaring what the plugin provides.

### What plugins can do

- Register custom tools that the LLM can invoke during sessions
- Add LLM providers with custom model discovery
- Intercept and modify chat messages, parameters, and headers before they reach the LLM
- React to session lifecycle events (start, end, switch, model change)
- Inject environment variables into shell commands
- Intercept tool execution (before and after)
- Modify tool definitions sent to the LLM
- Handle permission requests
- Add custom authentication flows (OAuth, API key)
- Stream and react to server events
- Modify the tinycode config at load time

### Plugin module shape

```typescript
import type { PluginModule } from "@tinycode/plugin"

export default {
  server: async (input, options) => {
    // Return a Hooks object
    return {
      // ... hooks go here
    }
  },
} satisfies PluginModule
```

The `PluginModule` type:

```typescript
type PluginModule = {
  id?: string              // Optional unique identifier
  server: Plugin           // Required: the plugin entry point
  tui?: never              // Server plugins cannot also be TUI plugins
  schema?: PluginSchema    // Optional: zod schema to validate plugin options
}
```

The current plugin API version is `1`. Plugins can declare compatibility in their `package.json`:

```json
{
  "engines": {
    "tinycode-plugin": "1"
  }
}
```

---

## Getting Started

### Minimal plugin structure

```
my-plugin/
  package.json
  src/
    index.ts
```

**package.json:**

```json
{
  "name": "my-tinycode-plugin",
  "version": "1.0.0",
  "type": "module",
  "exports": {
    "./server": "./src/index.ts"
  },
  "dependencies": {
    "@tinycode/plugin": "latest"
  }
}
```

The `./server` export is required for server-side plugins. If your plugin also extends the TUI, add a `./tui` export pointing to a file that exports a `TuiPlugin`.

**src/index.ts:**

```typescript
import type { PluginModule } from "@tinycode/plugin"
import { tool } from "@tinycode/plugin"

export default {
  server: async (input, options) => {
    return {
      tool: {
        greet: tool({
          description: "Greet someone by name",
          args: {
            name: tool.schema.string().describe("The name to greet"),
          },
          async execute(args) {
            return `Hello, ${args.name}!`
          },
        }),
      },
    }
  },
} satisfies PluginModule
```

### Install for local development

Use a `file://` path to install a plugin from your local filesystem:

```bash
tinycode plugin file:///path/to/my-plugin
```

Or add it directly to your config file (`~/.config/tinycode/config.json` or `.tinycode/config.json`):

```json
{
  "plugin": [
    "file:///path/to/my-plugin"
  ]
}
```

Plugins installed via `file://` paths are resolved directly without npm. This is the fastest way to iterate during development.

### PluginInput

The `server` function receives a `PluginInput` object:

```typescript
type PluginInput = {
  client: TinycodeClient   // SDK client connected to the running server
  project: Project          // Current project info (id, worktree, time)
  directory: string         // Current working directory
  worktree: string          // Project worktree root
  serverUrl: URL            // URL of the running tinycode server
  $: BunShell               // Bun shell for running commands
}
```

The `$` shell supports tagged template literals for running commands:

```typescript
const result = await input.$`ls -la ${input.directory}`.text()
```

---

## Hooks Reference

All hooks are optional fields on the `Hooks` object returned by your plugin's `server` function. Hooks follow two patterns:

1. **Observer hooks** receive an `input` object (read-only context) and an `output` object (mutable). Modify `output` to affect behavior.
2. **Registration hooks** are objects or functions that register capabilities (tools, providers, auth).

### Session Lifecycle

#### `session.start`

Fires when a new session is created.

```typescript
"session.start"?: (
  input: { sessionID: string; parentID?: string; agent?: string },
  output: {},
) => Promise<void>
```

- `parentID` is set when the session is a subagent spawned by another session.
- `agent` is the agent name if the session was started with a specific agent.

```typescript
"session.start": async (input) => {
  console.log(`Session started: ${input.sessionID}`)
  if (input.parentID) {
    console.log(`  Spawned by: ${input.parentID}`)
  }
},
```

#### `session.end`

Fires when a session is deleted.

```typescript
"session.end"?: (
  input: { sessionID: string },
  output: {},
) => Promise<void>
```

```typescript
"session.end": async (input) => {
  await cleanup(input.sessionID)
},
```

#### `session.switch`

Fires when the user switches to a different session.

```typescript
"session.switch"?: (
  input: { sessionID: string; previousSessionID?: string },
  output: {},
) => Promise<void>
```

```typescript
"session.switch": async (input) => {
  console.log(`Switched to ${input.sessionID} from ${input.previousSessionID}`)
},
```

#### `session.model.change`

Fires when the model is changed for a session.

```typescript
"session.model.change"?: (
  input: {
    sessionID: string
    providerID: string
    modelID: string
    previousModelID?: string
  },
  output: {},
) => Promise<void>
```

```typescript
"session.model.change": async (input) => {
  console.log(`Model changed to ${input.providerID}/${input.modelID}`)
},
```

### Chat Interception

#### `chat.message`

Fires when a new user message is received. Mutate `output` to modify the message or its parts before processing.

```typescript
"chat.message"?: (
  input: {
    sessionID: string
    agent?: string
    model?: { providerID: string; modelID: string }
    messageID?: string
    variant?: string
  },
  output: { message: UserMessage; parts: Part[] },
) => Promise<void>
```

```typescript
"chat.message": async (input, output) => {
  // Prepend context to every message
  output.parts.push({
    type: "text",
    text: `[Context: project=${input.agent}]`,
  })
},
```

#### `chat.params`

Modify the parameters sent to the LLM for a chat request. Mutate `output` fields to change temperature, token limits, and other model parameters.

```typescript
"chat.params"?: (
  input: {
    sessionID: string
    agent: string
    model: Model
    provider: ProviderContext
    message: UserMessage
  },
  output: {
    temperature: number
    topP: number
    topK: number
    maxOutputTokens: number | undefined
    options: Record<string, any>
  },
) => Promise<void>
```

The `provider` field contains:

```typescript
type ProviderContext = {
  source: "env" | "config" | "custom" | "api"
  info: Provider
  options: Record<string, any>
}
```

```typescript
"chat.params": async (input, output) => {
  // Force low temperature for the architect agent
  if (input.agent === "architect") {
    output.temperature = 0.1
  }
},
```

#### `chat.headers`

Add or modify HTTP headers sent with LLM API requests.

```typescript
"chat.headers"?: (
  input: {
    sessionID: string
    agent: string
    model: Model
    provider: ProviderContext
    message: UserMessage
  },
  output: { headers: Record<string, string> },
) => Promise<void>
```

```typescript
"chat.headers": async (input, output) => {
  output.headers["X-Custom-Header"] = "my-value"
},
```

### Tool Hooks

#### `tool`

Register custom tools. This is an object mapping tool names to `ToolDefinition` objects created with the `tool()` helper. See the [Tool API](#tool-api) section for details.

```typescript
tool?: {
  [key: string]: ToolDefinition
}
```

```typescript
tool: {
  "my-tool": tool({
    description: "Does something useful",
    args: {
      query: tool.schema.string().describe("Search query"),
    },
    async execute(args, context) {
      return `Result: ${args.query}`
    },
  }),
},
```

#### `tool.execute.before`

Fires before a tool executes. Mutate `output.args` to modify the arguments passed to the tool.

```typescript
"tool.execute.before"?: (
  input: { tool: string; sessionID: string; callID: string },
  output: { args: any },
) => Promise<void>
```

```typescript
"tool.execute.before": async (input, output) => {
  if (input.tool === "shell") {
    console.log(`Shell command: ${JSON.stringify(output.args)}`)
  }
},
```

#### `tool.execute.after`

Fires after a tool executes. Mutate `output` to modify the tool's result before it reaches the LLM.

```typescript
"tool.execute.after"?: (
  input: { tool: string; sessionID: string; callID: string; args: any },
  output: {
    title: string
    output: string
    metadata: any
  },
) => Promise<void>
```

```typescript
"tool.execute.after": async (input, output) => {
  // Truncate very long tool outputs
  if (output.output.length > 10000) {
    output.output = output.output.slice(0, 10000) + "\n[truncated]"
  }
},
```

#### `tool.definition`

Modify tool definitions (description and parameters) before they are sent to the LLM. Useful for customizing how tools are presented to models.

```typescript
"tool.definition"?: (
  input: { toolID: string },
  output: { description: string; parameters: any },
) => Promise<void>
```

```typescript
"tool.definition": async (input, output) => {
  if (input.toolID === "shell") {
    output.description += "\nPrefer using absolute paths."
  }
},
```

### Permission

#### `permission.ask`

Intercept permission requests. Mutate `output.status` to auto-allow or auto-deny permissions.

```typescript
"permission.ask"?: (
  input: Permission,
  output: { status: "ask" | "deny" | "allow" },
) => Promise<void>
```

```typescript
"permission.ask": async (input, output) => {
  // Auto-allow read operations
  if (input.tool === "read") {
    output.status = "allow"
  }
},
```

### Shell

#### `shell.env`

Inject environment variables into shell commands executed by tinycode.

```typescript
"shell.env"?: (
  input: { cwd: string; sessionID?: string; callID?: string },
  output: { env: Record<string, string> },
) => Promise<void>
```

```typescript
"shell.env": async (input, output) => {
  output.env["MY_PLUGIN_VAR"] = "some-value"
  output.env["NODE_OPTIONS"] = "--max-old-space-size=4096"
},
```

### Command

#### `command.execute.before`

Fires before a slash command executes. Mutate `output.parts` to inject additional context or modify the command input.

```typescript
"command.execute.before"?: (
  input: { command: string; sessionID: string; arguments: string },
  output: { parts: Part[] },
) => Promise<void>
```

```typescript
"command.execute.before": async (input, output) => {
  if (input.command === "ask") {
    output.parts.push({
      type: "text",
      text: "Additional context for the agent.",
    })
  }
},
```

### Auth

#### `auth`

Register a custom authentication flow for a provider. Supports both OAuth and API key methods.

```typescript
auth?: AuthHook
```

The `AuthHook` type:

```typescript
type AuthHook = {
  provider: string
  loader?: (auth: () => Promise<Auth>, provider: Provider) => Promise<Record<string, any>>
  methods: AuthMethod[]
}
```

Each method is either `type: "oauth"` or `type: "api"`:

**OAuth method:**

```typescript
{
  type: "oauth"
  label: string
  prompts?: AuthPrompt[]
  authorize(inputs?: Record<string, string>): Promise<AuthOAuthResult>
}
```

**API key method:**

```typescript
{
  type: "api"
  label: string
  prompts?: AuthPrompt[]
  authorize?(inputs?: Record<string, string>): Promise<
    | { type: "success"; key: string; provider?: string; metadata?: Record<string, string> }
    | { type: "failed" }
  >
}
```

Both method types support interactive prompts:

```typescript
type AuthPrompt =
  | {
      type: "text"
      key: string
      message: string
      placeholder?: string
      validate?: (value: string) => string | undefined
      when?: { key: string; op: "eq" | "neq"; value: string }
    }
  | {
      type: "select"
      key: string
      message: string
      options: Array<{ label: string; value: string; hint?: string }>
      when?: { key: string; op: "eq" | "neq"; value: string }
    }
```

```typescript
auth: {
  provider: "my-provider",
  methods: [
    {
      type: "api",
      label: "API Key",
      prompts: [
        {
          type: "text",
          key: "apiKey",
          message: "Enter your API key",
          placeholder: "sk-...",
          validate: (value) =>
            value.startsWith("sk-") ? undefined : "Key must start with sk-",
        },
      ],
      async authorize(inputs) {
        if (!inputs?.apiKey) return { type: "failed" }
        return {
          type: "success",
          key: inputs.apiKey,
        }
      },
    },
  ],
},
```

### Provider

#### `provider`

Register a custom LLM provider with model discovery.

```typescript
provider?: ProviderHook
```

```typescript
type ProviderHook = {
  id: string
  models?: (provider: ProviderV2, ctx: ProviderHookContext) => Promise<Record<string, ModelV2>>
}

type ProviderHookContext = {
  auth?: Auth
}
```

```typescript
provider: {
  id: "my-provider",
  async models(provider, ctx) {
    return {
      "my-model": {
        id: "my-model",
        name: "My Model",
        // ... model configuration
      },
    }
  },
},
```

### Config

#### `config`

Modify the tinycode configuration at load time. Receives the full config object (minus the `plugin` field).

```typescript
config?: (input: Config) => Promise<void>
```

The `Config` type is `Omit<SDKConfig, "plugin"> & { plugin?: Array<string | [string, PluginOptions]> }`.

```typescript
config: async (config) => {
  // Modify config at load time
},
```

### Event

#### `event`

Receive all server events. This is a firehose of every event emitted by the tinycode server.

```typescript
event?: (input: { event: Event }) => Promise<void>
```

```typescript
event: async ({ event }) => {
  if (event.type === "session.updated") {
    console.log(`Session updated: ${event.properties.sessionID}`)
  }
},
```

### Dispose

#### `dispose`

Called when the plugin is being unloaded. Use this to clean up resources, close connections, or flush data.

```typescript
dispose?: () => Promise<void>
```

```typescript
dispose: async () => {
  await db.close()
  console.log("Plugin cleaned up")
},
```

### Experimental Hooks

These hooks are prefixed with `experimental.` and may change in future versions without a major version bump. Use them with caution.

#### `experimental.chat.messages.transform`

Transform the full message history before it is sent to the LLM. Allows rewriting, filtering, or reordering messages.

```typescript
"experimental.chat.messages.transform"?: (
  input: {},
  output: {
    messages: {
      info: Message
      parts: Part[]
    }[]
  },
) => Promise<void>
```

#### `experimental.chat.system.transform`

Transform the system prompt before it is sent to the LLM.

```typescript
"experimental.chat.system.transform"?: (
  input: { sessionID?: string; model: Model },
  output: { system: string[] },
) => Promise<void>
```

```typescript
"experimental.chat.system.transform": async (input, output) => {
  output.system.push("Always respond in bullet points.")
},
```

#### `experimental.session.compacting`

Called before session compaction starts. Allows customizing or replacing the compaction prompt.

```typescript
"experimental.session.compacting"?: (
  input: { sessionID: string },
  output: { context: string[]; prompt?: string },
) => Promise<void>
```

- `context`: additional context strings appended to the default compaction prompt.
- `prompt`: if set, replaces the default compaction prompt entirely.

#### `experimental.compaction.autocontinue`

Called after compaction succeeds and before a synthetic user auto-continue message is added.

```typescript
"experimental.compaction.autocontinue"?: (
  input: {
    sessionID: string
    agent: string
    model: Model
    provider: ProviderContext
    message: UserMessage
    overflow: boolean
  },
  output: { enabled: boolean },
) => Promise<void>
```

Set `output.enabled = false` to skip the synthetic user "continue" turn after compaction.

#### `experimental.text.complete`

Called when a text part is complete. Allows post-processing of generated text.

```typescript
"experimental.text.complete"?: (
  input: { sessionID: string; messageID: string; partID: string },
  output: { text: string },
) => Promise<void>
```

---

## Tool API

Tools are the primary way plugins expose functionality to the LLM. Use the `tool()` helper from `@tinycode/plugin` to define tools with validated arguments and typed execution.

### Defining a tool

```typescript
import { tool } from "@tinycode/plugin"

const myTool = tool({
  description: "Search a knowledge base",
  args: {
    query: tool.schema.string().describe("The search query"),
    limit: tool.schema.number().optional().describe("Max results to return"),
  },
  async execute(args, context) {
    const results = await search(args.query, args.limit ?? 10)
    return {
      title: `Found ${results.length} results`,
      output: results.map((r) => r.summary).join("\n"),
    }
  },
})
```

### Argument schemas with `tool.schema`

`tool.schema` is a re-export of `zod` (`z`). Use it to define argument validation:

```typescript
args: {
  name: tool.schema.string().describe("User name"),
  age: tool.schema.number().int().min(0).describe("User age"),
  role: tool.schema.enum(["admin", "user"]).describe("User role"),
  tags: tool.schema.array(tool.schema.string()).optional().describe("Tags"),
}
```

Every argument field should include `.describe()` to document the parameter for the LLM.

### ToolContext

The `execute` function receives a `ToolContext` as its second argument:

```typescript
type ToolContext = {
  sessionID: string       // Current session ID
  messageID: string       // Current message ID
  agent: string           // Current agent name
  directory: string       // Current project directory for this session
  worktree: string        // Project worktree root
  abort: AbortSignal      // Abort signal for cancellation
  metadata(input: {       // Report metadata back to the UI
    title?: string
    metadata?: Record<string, any>
  }): void
  ask(input: AskInput): Promise<void>  // Request user permission
}
```

The `ask` function requests permission from the user:

```typescript
type AskInput = {
  permission: string            // Permission identifier
  patterns: string[]            // Patterns this permission covers
  always: string[]              // Patterns to auto-allow in future
  metadata: Record<string, any> // Additional context for the permission dialog
}
```

Use `metadata()` to update the tool's display title and pass structured data back:

```typescript
async execute(args, context) {
  context.metadata({ title: `Searching for "${args.query}"...` })
  const results = await search(args.query)
  context.metadata({
    title: `Found ${results.length} results`,
    metadata: { count: results.length },
  })
  return results.map((r) => r.text).join("\n")
}
```

### ToolResult

Tools can return either a plain string or a structured result:

```typescript
type ToolResult =
  | string
  | {
      title?: string                  // Display title in the UI
      output: string                  // Text output sent to the LLM
      metadata?: Record<string, any>  // Structured metadata
      attachments?: ToolAttachment[]  // File attachments
    }
```

### ToolAttachment

Attach files to tool results:

```typescript
type ToolAttachment = {
  type: "file"
  mime: string        // MIME type (e.g. "image/png", "application/json")
  url: string         // File URL or data URI
  filename?: string   // Optional display filename
}
```

```typescript
async execute(args) {
  const chart = await generateChart(args.data)
  return {
    output: "Chart generated successfully.",
    attachments: [
      {
        type: "file",
        mime: "image/png",
        url: `file://${chart.path}`,
        filename: "chart.png",
      },
    ],
  }
}
```

---

## Configuration

Users configure plugins in their tinycode config file (`~/.config/tinycode/config.json` for global, or `.tinycode/config.json` for project-level):

```json
{
  "plugin": [
    "my-plugin-name",
    ["my-other-plugin", { "apiKey": "sk-...", "verbose": true }]
  ]
}
```

Plugins listed as a plain string receive no options. Plugins listed as a `[name, options]` tuple receive the options object as the second argument to the `server` function.

### Schema validation

Plugins can export a `schema` field on their `PluginModule` to validate options before the plugin loads. The schema is a zod type:

```typescript
import { z } from "zod"
import type { PluginModule } from "@tinycode/plugin"

export default {
  schema: z.object({
    apiKey: z.string().min(1, "API key is required"),
    verbose: z.boolean().optional().default(false),
  }),
  server: async (input, options) => {
    // options is validated against the schema before reaching here
    const { apiKey, verbose } = options as { apiKey: string; verbose: boolean }
    return {}
  },
} satisfies PluginModule
```

When validation fails, tinycode reports the error and skips loading the plugin.

---

## Testing

The `@tinycode/plugin/test` module provides utilities for testing plugins without a running tinycode server.

### `createMockPluginInput`

Creates a mock `PluginInput` with sensible defaults. All fields can be overridden:

```typescript
import { createMockPluginInput } from "@tinycode/plugin/test"

const input = createMockPluginInput({
  directory: "/my/project",
  worktree: "/my/project",
})
```

Default values:
- `client`: empty object stub
- `project`: `{ id: "test-project", worktree: "/tmp/tinycode-plugin-test", time: { created: Date.now() } }`
- `directory`: `"/tmp/tinycode-plugin-test"`
- `worktree`: `"/tmp/tinycode-plugin-test"`
- `serverUrl`: `new URL("http://localhost:4096")`
- `$`: no-op shell that returns empty results

### `createMockToolContext`

Creates a mock `ToolContext` for testing tool execution:

```typescript
import { createMockToolContext } from "@tinycode/plugin/test"

const context = createMockToolContext({
  sessionID: "my-session",
  agent: "build",
})
```

Default values:
- `sessionID`: `"test-session"`
- `messageID`: `"test-message"`
- `agent`: `"test-agent"`
- `directory`: `"/tmp/tinycode-plugin-test"`
- `worktree`: `"/tmp/tinycode-plugin-test"`
- `abort`: `AbortSignal.abort()`
- `metadata`: no-op function
- `ask`: no-op async function

### `createTestHarness`

Loads a `PluginModule` with mock input and returns the resolved hooks plus a typed `invoke` helper:

```typescript
import { createTestHarness } from "@tinycode/plugin/test"
import plugin from "./index"

const { hooks, invoke } = await createTestHarness(plugin, {
  input: { directory: "/my/project" },
  pluginOptions: { apiKey: "test-key" },
})
```

The `invoke` function calls a hook by name with full type checking:

```typescript
await invoke(
  "session.start",
  { sessionID: "s1", parentID: undefined, agent: "build" },
  {},
)
```

If the hook is not registered, `invoke` throws an error.

### Example test

```typescript
import { describe, it, expect } from "bun:test"
import { createTestHarness, createMockToolContext } from "@tinycode/plugin/test"
import plugin from "../src/index"

describe("my-plugin", () => {
  it("registers the greet tool", async () => {
    const { hooks } = await createTestHarness(plugin)
    expect(hooks.tool).toBeDefined()
    expect(hooks.tool!.greet).toBeDefined()
  })

  it("greet tool returns greeting", async () => {
    const { hooks } = await createTestHarness(plugin)
    const context = createMockToolContext()
    const result = await hooks.tool!.greet.execute({ name: "World" }, context)
    expect(result).toBe("Hello, World!")
  })

  it("session.start hook fires", async () => {
    const sessions: string[] = []
    // Test with a plugin that tracks sessions
    const { invoke } = await createTestHarness(plugin)
    await invoke(
      "session.start",
      { sessionID: "test-1", agent: "build" },
      {},
    )
  })
})
```

---

## Publishing

### Package requirements

Your `package.json` must include:

1. **`exports`** with a `./server` entry pointing to your plugin module:

   ```json
   {
     "exports": {
       "./server": "./src/index.ts"
     }
   }
   ```

   If your plugin has a TUI component, also include `./tui`:

   ```json
   {
     "exports": {
       "./server": "./src/index.ts",
       "./tui": "./src/tui.ts"
     }
   }
   ```

2. **`@tinycode/plugin`** as a dependency for type support and the `tool()` helper.

3. **`type: "module"`** since tinycode plugins are ESM.

### Publishing to npm

```bash
npm publish
```

Users install your plugin with:

```bash
tinycode plugin your-package-name
```

### The plugin registry

Tinycode maintains a curated plugin registry. Registry plugins can be installed by short name instead of full npm specifier:

```bash
tinycode plugin azure           # Resolves to @tinycode/plugin-azure
tinycode plugin github-copilot  # Resolves to @tinycode/plugin-github-copilot
```

Users can search the registry:

```bash
tinycode plugin-search provider
tinycode plugin-search linter
```

To submit your plugin to the registry, open a pull request adding an entry to `src/plugin/registry.json`:

```json
{
  "name": "your-plugin",
  "npm": "@yourorg/tinycode-plugin-name",
  "description": "Short description of what it does",
  "author": "your-name",
  "tags": ["relevant", "tags"]
}
```

---

## Complete Example

A full working plugin that provides a tool, reacts to session events, validates its configuration, and cleans up on dispose.

**package.json:**

```json
{
  "name": "tinycode-plugin-metrics",
  "version": "1.0.0",
  "type": "module",
  "exports": {
    "./server": "./src/index.ts"
  },
  "dependencies": {
    "@tinycode/plugin": "latest"
  }
}
```

**src/index.ts:**

```typescript
import { z } from "zod"
import type { PluginModule, PluginInput, PluginOptions } from "@tinycode/plugin"
import { tool } from "@tinycode/plugin"

type SessionMetrics = {
  startTime: number
  toolCalls: number
  modelChanges: number
}

export default {
  schema: z.object({
    endpoint: z.string().url("Must be a valid URL"),
    verbose: z.boolean().optional().default(false),
  }),

  server: async (input: PluginInput, options?: PluginOptions) => {
    const config = options as { endpoint: string; verbose: boolean }
    const sessions = new Map<string, SessionMetrics>()

    async function flush(sessionID: string) {
      const metrics = sessions.get(sessionID)
      if (!metrics) return
      if (config.verbose) {
        console.log(`Flushing metrics for ${sessionID}:`, metrics)
      }
      await fetch(config.endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ sessionID, ...metrics }),
      }).catch(() => {})
      sessions.delete(sessionID)
    }

    return {
      tool: {
        metrics: tool({
          description: "Show session metrics (tool call count, duration, model changes)",
          args: {
            sessionID: tool.schema.string().optional().describe(
              "Session ID to query. Defaults to current session."
            ),
          },
          async execute(args, context) {
            const id = args.sessionID ?? context.sessionID
            const metrics = sessions.get(id)
            if (!metrics) {
              return "No metrics found for this session."
            }
            const duration = Math.round((Date.now() - metrics.startTime) / 1000)
            return {
              title: "Session Metrics",
              output: [
                `Duration: ${duration}s`,
                `Tool calls: ${metrics.toolCalls}`,
                `Model changes: ${metrics.modelChanges}`,
              ].join("\n"),
              metadata: { duration, ...metrics },
            }
          },
        }),
      },

      "session.start": async (input) => {
        sessions.set(input.sessionID, {
          startTime: Date.now(),
          toolCalls: 0,
          modelChanges: 0,
        })
      },

      "session.end": async (input) => {
        await flush(input.sessionID)
      },

      "session.model.change": async (input) => {
        const metrics = sessions.get(input.sessionID)
        if (metrics) {
          metrics.modelChanges += 1
        }
      },

      "tool.execute.after": async (input) => {
        const metrics = sessions.get(input.sessionID)
        if (metrics) {
          metrics.toolCalls += 1
        }
      },

      dispose: async () => {
        // Flush all remaining sessions
        for (const sessionID of sessions.keys()) {
          await flush(sessionID)
        }
        sessions.clear()
      },
    }
  },
} satisfies PluginModule
```

Install for development:

```bash
tinycode plugin file:///path/to/tinycode-plugin-metrics
```

Or configure with options in `~/.config/tinycode/config.json`:

```json
{
  "plugin": [
    ["tinycode-plugin-metrics", {
      "endpoint": "https://metrics.example.com/collect",
      "verbose": true
    }]
  ]
}
```

---

## OpenCode Plugin Compatibility

The following OpenCode plugins have been **deprecated** because their functionality is now built into tinycode:

| OpenCode Plugin | Built-in Replacement | Status |
|---|---|---|
| `opencode-openai-codex-auth` | Built-in Codex auth | Deprecated since 1.14 |
| `opencode-copilot-auth` | Built-in Copilot auth | Deprecated since 1.14 |
| `opencode-gitlab-auth` | Built-in GitLab auth | Deprecated since 1.18 |
| `opencode-poe-auth` | Built-in Poe auth | Deprecated since 1.18 |

### Migration

If you have any of these plugins in your `config.json` under `plugin_origins`, you can safely remove them. tinycode silently skips deprecated plugins, so no action is required — but removing them avoids unnecessary startup warnings.

OpenCode plugins that use a different hook interface than tinycode's `Hooks` type are not supported. If you have a custom OpenCode plugin, migrate it to the tinycode plugin SDK (`tinycode-plugin` on npm). See the rest of this guide for the tinycode plugin API.
