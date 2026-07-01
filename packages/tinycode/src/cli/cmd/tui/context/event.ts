import type { Event as SDKEvent } from "@tinycode/sdk/v2"
import type { SessionEvent } from "@/core/session-event"
import { useProject } from "./project"
import { useSDK } from "./sdk"

// SessionEvent types have .data but the wire format has .properties
type SessionEventAsWireFormat<T extends SessionEvent.Event> = Omit<T, "data"> & {
  properties: T["data"]
}

type Event = SDKEvent | SessionEventAsWireFormat<SessionEvent.Event>

type EventMetadata = {
  workspace: string | undefined
}

export function useEvent() {
  const project = useProject()
  const sdk = useSDK()

  function subscribe(handler: (event: Event, metadata: EventMetadata) => void) {
    return sdk.event.on("event", (event) => {
      if (event.payload.type === "sync") {
        return
      }

      if (event.directory === "global" || event.project === project.project()) {
        handler(event.payload, { workspace: event.workspace })
      }
    })
  }

  function on<T extends Event["type"]>(
    type: T,
    handler: (event: Extract<Event, { type: T }>, metadata: EventMetadata) => void,
  ) {
    return subscribe((event: Event, metadata: EventMetadata) => {
      if (event.type !== type) return
      handler(event as Extract<Event, { type: T }>, metadata)
    })
  }

  return {
    subscribe,
    on,
  }
}
