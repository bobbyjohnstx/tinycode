import { spawn } from "child_process"
import path from "path"
import { fileURLToPath } from "url"
import { cmd } from "./cmd"

const OMT_COORDINATOR = path.join(
  path.dirname(fileURLToPath(import.meta.url)),
  "../../../../../../oh-my-tiny/dist/team/coordinator.js",
)

export const OmtCommand = cmd({
  command: "omt",
  describe: "oh-my-tiny team orchestration via tinycode sessions",
  builder: (yargs) =>
    yargs
      .option("team", {
        type: "string",
        describe: "Team name",
        default: "omt-team",
      })
      .option("workers", {
        type: "number",
        describe: "Number of worker sessions",
        default: 2,
      })
      .option("task", {
        type: "string",
        describe: "Task description to assign",
        demandOption: true,
      })
      .option("server-url", {
        type: "string",
        describe: "Tinycode server URL",
        default: "http://127.0.0.1:4096",
      })
      .option("cwd", {
        type: "string",
        describe: "Project directory",
        default: process.cwd(),
      }),
  handler: async (args) => {
    const coordinatorArgs = [
      OMT_COORDINATOR,
      "--team", args.team as string,
      "--workers", String(args.workers),
      "--task", args.task as string,
      "--server-url", args["server-url"] as string,
      "--cwd", args.cwd as string,
    ]

    console.log(`omt: delegating to oh-my-tiny coordinator`)
    console.log(`     node ${coordinatorArgs.join(" ")}`)

    const proc = spawn("node", coordinatorArgs, { stdio: "inherit" })
    await new Promise<void>((resolve, reject) => {
      proc.on("exit", (code) => {
        if (code === 0) resolve()
        else reject(new Error(`coordinator exited with code ${code}`))
      })
    })
  },
})
