import type { PluginInput, PluginModule, Hooks } from "./index.js"
import type { ToolContext } from "./tool.js"

/**
 * Create a mock `PluginInput` with sensible defaults for testing.
 * All fields can be overridden via the `overrides` parameter.
 */
export function createMockPluginInput(
  overrides?: Partial<PluginInput>,
): PluginInput {
  return {
    client: {} as PluginInput["client"],
    project: {
      id: "test-project",
      worktree: "/tmp/tinycode-plugin-test",
      time: { created: Date.now() },
    },
    directory: "/tmp/tinycode-plugin-test",
    worktree: "/tmp/tinycode-plugin-test",
    serverUrl: new URL("http://localhost:4096"),
    $: (() => {
      const shell = (() =>
        Promise.resolve({
          stdout: Buffer.from(""),
          stderr: Buffer.from(""),
          exitCode: 0,
          text: () => "",
          json: () => ({}),
          arrayBuffer: () => new ArrayBuffer(0),
          bytes: () => new Uint8Array(0),
          blob: () => new Blob(),
        })) as unknown as PluginInput["$"]
      shell.braces = () => []
      shell.escape = (input: string) => input
      shell.env = () => shell
      shell.cwd = () => shell
      shell.nothrow = () => shell
      shell.throws = () => shell
      return shell
    })(),
    ...overrides,
  }
}

/**
 * Create a mock `ToolContext` with sensible defaults for testing.
 * All fields can be overridden via the `overrides` parameter.
 */
export function createMockToolContext(
  overrides?: Partial<ToolContext>,
): ToolContext {
  return {
    sessionID: "test-session",
    messageID: "test-message",
    agent: "test-agent",
    directory: "/tmp/tinycode-plugin-test",
    worktree: "/tmp/tinycode-plugin-test",
    abort: AbortSignal.abort(),
    metadata: () => {},
    ask: async () => {},
    progress: () => {},
    messages: async () => Object.freeze([]),
    sessionInfo: async () => Object.freeze({ id: "test-session", model: "test-model", agent: "test-agent" }),
    ...overrides,
  }
}

/**
 * Load a `PluginModule` with mock input and return the resolved hooks.
 * Optionally accepts `PluginInput` overrides and plugin options.
 */
export async function createTestHarness(
  pluginModule: PluginModule,
  options?: {
    input?: Partial<PluginInput>
    pluginOptions?: Record<string, unknown>
  },
): Promise<{
  hooks: Hooks
  invoke: <K extends keyof Hooks>(
    hook: K,
    ...args: Hooks[K] extends (...a: infer A) => any ? A : never
  ) => Hooks[K] extends (...a: any[]) => infer R ? R : never
}> {
  const input = createMockPluginInput(options?.input)
  const hooks = await pluginModule.server(input, options?.pluginOptions)

  return {
    hooks,
    invoke: ((hook: string, ...args: any[]) => {
      const fn = hooks[hook as keyof Hooks]
      if (typeof fn !== "function") {
        throw new Error(`Hook "${hook}" is not registered`)
      }
      return (fn as Function)(...args)
    }) as any,
  }
}
