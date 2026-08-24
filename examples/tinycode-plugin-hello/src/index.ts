import type { PluginModule } from "tinycode-plugin"
import { tool } from "tinycode-plugin/tool"
import { z } from "zod"

const schema = z.object({
  greeting: z.string().default("Hello"),
})

export default {
  schema,
  server: async (input, options) => {
    const config = schema.parse(options ?? {})

    return {
      tool: {
        hello_world: tool({
          description: "Say hello to someone",
          args: {
            name: tool.schema.string().describe("Name to greet"),
          },
          execute: async (args) => {
            return `${config.greeting}, ${args.name}! Welcome to tinycode plugins.`
          },
        }),

        hello_time: tool({
          description: "Get the current time with a greeting",
          args: {
            timezone: tool.schema
              .string()
              .describe("IANA timezone (e.g. America/New_York)")
              .optional(),
          },
          execute: async (args, context) => {
            context.progress("Checking the time...")
            const tz = args.timezone ?? "UTC"
            const now = new Date().toLocaleString("en-US", { timeZone: tz })
            return `${config.greeting}! The current time in ${tz} is ${now}.`
          },
        }),
      },

      "session.start": async (event) => {
        console.log(
          `[hello-plugin] Session ${event.sessionID} started (agent: ${event.agent})`,
        )
      },

      "session.model.change": async (event) => {
        console.log(
          `[hello-plugin] Model changed to ${event.modelID} in session ${event.sessionID}`,
        )
      },
    }
  },
} satisfies PluginModule
