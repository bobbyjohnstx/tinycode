export type KeyHandlerState = {
  mode: "normal" | "shell"
  popover: "at" | "slash" | "ask" | null
  working: boolean
  cursorPosition: number
  textLength: number
  isImeComposing: boolean
}

export type KeyEvent = {
  key: string
  shiftKey?: boolean
  ctrlKey?: boolean
  metaKey?: boolean
  altKey?: boolean
  repeat?: boolean
  code?: string
}

export type KeyAction =
  | "shell-mode"
  | "close-popover"
  | "normal-mode"
  | "abort"
  | "insert-newline"
  | "submit"
  | "file-pick"
  | "popover-nav"
  | "blur"
  | "pass-through"

/**
 * Pure decision function modeling the handleKeyDown logic from prompt-input.tsx.
 * Returns the action that handleKeyDown would take for the given state + event.
 */
export function keyHandlerDecision(state: KeyHandlerState, event: KeyEvent): KeyAction {
  const ctrl = (event.ctrlKey && !event.metaKey && !event.altKey && !event.shiftKey) ?? false
  const mod = ((event.metaKey || event.ctrlKey) && !event.altKey && !event.shiftKey) ?? false

  // Ctrl/Cmd+U -> file picker
  if (mod && event.key.toLowerCase() === "u") {
    if (state.mode !== "normal") return "pass-through"
    return "file-pick"
  }

  // ! at position 0 in normal mode -> shell mode
  if (event.key === "!" && state.mode === "normal" && state.cursorPosition === 0) {
    return "shell-mode"
  }

  // Escape handling
  if (event.key === "Escape") {
    if (state.popover) return "close-popover"
    if (state.mode === "shell") return "normal-mode"
    if (state.working) return "abort"
    return "pass-through"
  }

  // Backspace in shell mode at position 0 with empty text -> normal mode
  if (
    event.key === "Backspace" &&
    state.mode === "shell" &&
    state.cursorPosition === 0 &&
    state.textLength === 0
  ) {
    return "normal-mode"
  }

  // Shift+Enter -> insert newline (handled before IME check)
  if (event.key === "Enter" && event.shiftKey) {
    return "insert-newline"
  }

  // Enter during IME composition -> pass through
  if (event.key === "Enter" && state.isImeComposing) {
    return "pass-through"
  }

  // Popover navigation
  if (state.popover) {
    const nav =
      event.key === "ArrowUp" ||
      event.key === "ArrowDown" ||
      event.key === "Enter" ||
      event.key === "Tab"
    const ctrlNav = ctrl && (event.key === "n" || event.key === "p")
    if (nav || ctrlNav) return "popover-nav"
  }

  // Ctrl+G -> close popover or abort
  if (ctrl && event.code === "KeyG") {
    if (state.popover) return "close-popover"
    if (state.working) return "abort"
    return "pass-through"
  }

  // Enter (non-shift, non-IME) -> submit
  if (event.key === "Enter" && !event.shiftKey) {
    if (event.repeat) return "pass-through"
    return "submit"
  }

  return "pass-through"
}
