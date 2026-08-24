# Adding a Tool

There are two ways to add a tool to tinycode. Choose based on your comfort level with the codebase.

## Path 1: Plugin Tool (Recommended for Contributors)

Plugin tools use the `tinycode-plugin` SDK (npm: `tinycode-plugin`). No Effect framework knowledge required.

Tools are registered inside your plugin's `server` function return object:

```typescript
import type { PluginModule } from "tinycode-plugin"
import { tool } from "tinycode-plugin/tool"

export default {
  server: async (input) => ({
    tool: {
      my_tool: tool({
        description: "A short description of what this tool does",
        args: {
          query: tool.schema.string().describe("The search query"),
          limit: tool.schema.number().optional().describe("Max results to return"),
        },
        execute: async (args, context) => {
          const results = await doSomething(args.query, args.limit ?? 10)
          return {
            output: JSON.stringify(results, null, 2),
          }
        },
      }),
    },
  }),
} satisfies PluginModule
```

Key points:

- Use `tool.schema` (which is zod's `z`) to define `args` as a flat shape -- descriptions are surfaced to the LLM
- Return a string or an object with an `output` string (optionally with `title`, `metadata`, `attachments`)
- The `context` parameter provides `sessionID`, `directory`, `progress()`, `ask()`, `messages()`, and `sessionInfo()`
- Plugin tools are loaded via dynamic import and do not have access to tinycode internals

See [plugin-development.md](plugin-development.md) for the full plugin development guide, including how to register, test, and publish plugins.

## Path 2: Core Tool (Requires Effect Fluency)

Core tools live in `packages/tinycode/src/tool/` and use the Effect framework for typed errors, dependency injection, and resource management.

### Structure

Each core tool consists of two files side by side:

- `my-tool.ts` -- the tool implementation
- `my-tool.txt` -- the LLM-facing description (imported as `DESCRIPTION`)

### Pattern

Core tools follow the `Tool.define()` pattern with Effect generators:

```typescript
import { Effect, Schema } from "effect"
import DESCRIPTION from "./my-tool.txt"
import * as Tool from "./tool"

export const Parameters = Schema.Struct({
  query: Schema.String.annotate({ description: "The search query" }),
  limit: Schema.optional(Schema.Number).annotate({
    description: "Max results to return",
  }),
})

export const MyTool = Tool.define(
  "my-tool",
  Effect.gen(function* () {
    // Resolve dependencies from the Effect context
    // e.g.: const fs = yield* AppFileSystem.Service

    return {
      description: DESCRIPTION,
      parameters: Parameters,
      execute: (params: { query: string; limit?: number }, ctx: Tool.Context) =>
        Effect.gen(function* () {
          // Request permission if needed
          yield* ctx.ask({
            permission: "my-tool",
            patterns: [params.query],
            always: ["*"],
            metadata: { query: params.query },
          })

          // Tool logic here
          const result = yield* doSomething(params.query, params.limit ?? 10)

          return {
            title: params.query,
            metadata: { count: result.length },
            output: result.join("\n"),
          }
        }).pipe(Effect.orDie),
    }
  }),
)
```

### Reference

Look at these existing tools for examples:

- `glob.ts` -- simple tool with file system access and permission handling
- `grep.ts` -- similar pattern with streaming results
- `read.ts` -- file reading with content truncation

### Registering a Core Tool

After creating the tool files, register it in `packages/tinycode/src/tool/registry.ts` so the session processor discovers it.

## Which Path Should I Choose?

| | Plugin tool | Core tool |
|---|---|---|
| Difficulty | Low | High |
| Prerequisites | TypeScript, zod | TypeScript, Effect framework |
| Access to internals | No | Yes (file system, LSP, storage) |
| Deployment | npm package | Part of tinycode binary |
| Best for | Self-contained utilities | Tools needing deep integration |
