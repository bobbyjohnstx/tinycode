export * from "./client.js"
export * from "./server.js"

import { createTinycodeClient } from "./client.js"
import { createTinycodeServer } from "./server.js"
import type { ServerOptions } from "./server.js"

export * as data from "./data.js"

export async function createTinycode(options?: ServerOptions) {
  const server = await createTinycodeServer({
    ...options,
  })

  const client = createTinycodeClient({
    baseUrl: server.url,
  })

  return {
    client,
    server,
  }
}
