import { Effect } from "effect"
import { Server } from "../../server/server"
import { effectCmd } from "../effect-cmd"
import { withNetworkOptions, resolveNetworkOptions } from "../network"
import { Flag } from "@/core/flag/flag"

export const ServeCommand = effectCmd({
  command: "serve",
  builder: (yargs) => withNetworkOptions(yargs),
  describe: "starts a headless tinycode server",
  // Server loads instances per-request via x-tinycode-directory header — no
  // need for an ambient project InstanceContext at startup.
  instance: false,
  handler: Effect.fn("Cli.serve")(function* (args) {
    const opts = yield* resolveNetworkOptions(args)
    if (!Flag.TINYCODE_SERVER_PASSWORD) {
      const host = opts.hostname
      if (host === "0.0.0.0" || host === "::" || (host !== "127.0.0.1" && host !== "localhost" && host !== "::1")) {
        console.error("Error: TINYCODE_SERVER_PASSWORD is required when binding to a non-loopback address.")
        process.exit(1)
      }
      console.log("Warning: TINYCODE_SERVER_PASSWORD is not set; server is unsecured (loopback only).")
    }
    const server = yield* Effect.promise(() => Server.listen(opts))
    console.log(`tinycode server listening on http://${server.hostname}:${server.port}`)

    yield* Effect.never
  }),
})
