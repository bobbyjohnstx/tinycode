import { Schema } from "effect"
import { PositiveInt } from "@/core/schema"

export const Server = Schema.Struct({
  port: Schema.optional(PositiveInt).annotate({
    description: "Port to listen on",
  }),
  hostname: Schema.optional(Schema.String).annotate({ description: "Hostname to listen on" }),
  mdns: Schema.optional(Schema.Boolean).annotate({ description: "Enable mDNS service discovery" }),
  mdnsDomain: Schema.optional(Schema.String).annotate({
    description: "Custom domain name for mDNS service (default: tinycode.local)",
  }),
  cors: Schema.optional(Schema.mutable(Schema.Array(Schema.String))).annotate({
    description: "Additional domains to allow for CORS",
  }),
  max_instances: Schema.optional(PositiveInt).annotate({
    description: "Maximum cached project instances. Least-recently-used instances are evicted when exceeded. Default: 32.",
  }),
  max_sessions: Schema.optional(PositiveInt).annotate({
    description: "Maximum concurrent active sessions (busy/processing). New session prompts are rejected when exceeded. Default: unlimited.",
  }),
}).annotate({ identifier: "ServerConfig" })
export type Server = Schema.Schema.Type<typeof Server>

export * as ConfigServer from "./server"
