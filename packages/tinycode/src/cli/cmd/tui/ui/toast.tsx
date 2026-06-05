import { createContext, useContext, type ParentProps, Show } from "solid-js"
import { createStore } from "solid-js/store"
import { useTheme } from "@tui/context/theme"
import { useTerminalDimensions } from "@opentui/solid"
import { SplitBorder } from "../component/border"
import { TextAttributes } from "@opentui/core"
import { Schema } from "effect"
import { TuiEvent } from "../event"

type ToastInput = Schema.Codec.Encoded<typeof TuiEvent.ToastShow.properties>
export type ToastOptions = Schema.Schema.Type<typeof TuiEvent.ToastShow.properties>

const decodeToastOptions = Schema.decodeUnknownSync(TuiEvent.ToastShow.properties)

export function Toast() {
  const toast = useToast()
  const { theme } = useTheme()
  const dimensions = useTerminalDimensions()

  return (
    <Show when={toast.currentToast()}>
      {(current) => (
        <box
          position="absolute"
          justifyContent="center"
          alignItems="flex-start"
          top={2}
          right={2}
          maxWidth={Math.min(60, dimensions().width - 6)}
          paddingLeft={2}
          paddingRight={2}
          paddingTop={1}
          paddingBottom={1}
          backgroundColor={theme.backgroundPanel}
          borderColor={theme[current().variant]}
          border={["left", "right"]}
          customBorderChars={SplitBorder.customBorderChars}
        >
          <Show when={current().title}>
            <text attributes={TextAttributes.BOLD} marginBottom={1} fg={theme.text}>
              {current().title}
            </text>
          </Show>
          <text fg={theme.text} wrapMode="word" width="100%">
            {current().message}
          </text>
        </box>
      )}
    </Show>
  )
}

function init() {
  const [store, setStore] = createStore({
    queue: [] as ToastOptions[],
  })

  let timeoutHandle: NodeJS.Timeout | null = null

  function showNext() {
    if (store.queue.length === 0) return
    const current = store.queue[0]
    if (timeoutHandle) clearTimeout(timeoutHandle)
    timeoutHandle = setTimeout(() => {
      setStore("queue", (queue) => queue.slice(1))
      showNext()
    }, current.duration).unref()
  }

  const toast = {
    show(options: ToastInput) {
      const toastOptions = decodeToastOptions(options)
      setStore("queue", (queue) => {
        const newQueue = [...queue, toastOptions]
        return newQueue.length > 5 ? newQueue.slice(-5) : newQueue
      })
      if (store.queue.length === 1) {
        showNext()
      }
    },
    error: (err: any) => {
      if (err instanceof Error)
        return toast.show({
          variant: "error",
          message: err.message,
        })
      toast.show({
        variant: "error",
        message: "An unknown error has occurred",
      })
    },
    currentToast(): ToastOptions | null {
      return store.queue[0] ?? null
    },
  }
  return toast
}

export type ToastContext = ReturnType<typeof init>

const ctx = createContext<ToastContext>()

export function ToastProvider(props: ParentProps) {
  const value = init()
  return <ctx.Provider value={value}>{props.children}</ctx.Provider>
}

export function useToast() {
  const value = useContext(ctx)
  if (!value) {
    throw new Error("useToast must be used within a ToastProvider")
  }
  return value
}
