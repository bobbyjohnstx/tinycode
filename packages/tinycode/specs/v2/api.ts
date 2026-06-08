// @ts-nocheck

import { OpenCode } from "@/core"
import { ReadTool } from "@/core/tools"

const tc = OpenCode.make({})

tc.tool.add(ReadTool)

tc.tool.add({
  name: "bash",
  schema: {
    type: "object",
    properties: {
      command: {
        type: "string",
        description: "The command to run.",
      },
    },
    required: ["command"],
  },
  execute(input, ctx) {},
})

tc.auth.add({
  provider: "openai",
  type: "api",
  value: process.env.OPENAI_API_KEY,
})

tc.agent.add({
  name: "build",
  permissions: [],
  model: {
    id: "gpt-5-5",
    provider: "openai",
    variant: "xhigh",
  },
})

const sessionID = await tc.session.create({
  agent: "build",
})

tc.subscribe((event) => {
  console.log(event)
})

await tc.session.prompt({
  sessionID,
  text: "hey what is up",
})

await tc.session.prompt({
  sessionID,
  text: "what is up with this",
  files: [
    {
      mime: "image/png",
      uri: "data:image/png;base64,xxxx",
    },
  ],
})

await tc.session.wait()

console.log(await tc.session.messages(sessionID))
