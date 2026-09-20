import { describe, expect, test } from "bun:test"
import { keyHandlerDecision, type KeyHandlerState } from "./key-handler-decision"

// ---------------------------------------------------------------------------
// Autocomplete trigger patterns — DUPLICATED from prompt-input.tsx handleInput
// (lines ~940-942). These patterns are inline in the SolidJS component and
// cannot be imported directly. If the production patterns change, these tests
// will silently pass against stale copies.
//
// TODO(#177): Extract patterns to a shared module so tests import the real ones.
// Source: prompt-input.tsx handleInput() — slashMatch, atMatch, askMatch
// ---------------------------------------------------------------------------
const slashPattern = /^\/(\S*)$/
const atPattern = /@(\S*)$/
const askPattern = /^\/ask\s+(\S*)$/

// ---------------------------------------------------------------------------
// Tests: handleKeyDown decision logic
// ---------------------------------------------------------------------------
describe("handleKeyDown decision logic", () => {
  const baseState: KeyHandlerState = {
    mode: "normal",
    popover: null,
    working: false,
    cursorPosition: 0,
    textLength: 0,
    isImeComposing: false,
  }

  test("exclamation at cursor position 0 in normal mode sets mode to shell", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "normal", cursorPosition: 0 },
      { key: "!" },
    )
    expect(result).toBe("shell-mode")
  })

  test("exclamation NOT at cursor position 0 passes through to typing", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "normal", cursorPosition: 5 },
      { key: "!" },
    )
    expect(result).toBe("pass-through")
  })

  test("exclamation in shell mode passes through regardless of position", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell", cursorPosition: 0 },
      { key: "!" },
    )
    expect(result).toBe("pass-through")
  })

  test("enter without shift submits the current input", () => {
    const result = keyHandlerDecision(baseState, { key: "Enter" })
    expect(result).toBe("submit")
  })

  test("shift+enter inserts a newline instead of submitting", () => {
    const result = keyHandlerDecision(baseState, { key: "Enter", shiftKey: true })
    expect(result).toBe("insert-newline")
  })

  test("escape with popover open closes the popover", () => {
    const result = keyHandlerDecision(
      { ...baseState, popover: "slash" },
      { key: "Escape" },
    )
    expect(result).toBe("close-popover")
  })

  test("escape in shell mode resets to normal mode", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell" },
      { key: "Escape" },
    )
    expect(result).toBe("normal-mode")
  })

  test("escape while working triggers abort", () => {
    const result = keyHandlerDecision(
      { ...baseState, working: true },
      { key: "Escape" },
    )
    expect(result).toBe("abort")
  })

  test("escape with no popover, normal mode, and not working passes through", () => {
    const result = keyHandlerDecision(baseState, { key: "Escape" })
    expect(result).toBe("pass-through")
  })

  test("backspace in shell mode at position 0 with empty text resets to normal mode", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell", cursorPosition: 0, textLength: 0 },
      { key: "Backspace" },
    )
    expect(result).toBe("normal-mode")
  })

  test("backspace in shell mode with non-empty text passes through", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell", cursorPosition: 2, textLength: 5 },
      { key: "Backspace" },
    )
    expect(result).toBe("pass-through")
  })

  test("enter during IME composition passes through", () => {
    const result = keyHandlerDecision(
      { ...baseState, isImeComposing: true },
      { key: "Enter" },
    )
    expect(result).toBe("pass-through")
  })

  test("shift+enter during IME composition still inserts newline", () => {
    const result = keyHandlerDecision(
      { ...baseState, isImeComposing: true },
      { key: "Enter", shiftKey: true },
    )
    expect(result).toBe("insert-newline")
  })

  test("repeated enter key does not submit", () => {
    const result = keyHandlerDecision(baseState, { key: "Enter", repeat: true })
    expect(result).toBe("pass-through")
  })

  test("ctrl+g with popover open closes popover", () => {
    const result = keyHandlerDecision(
      { ...baseState, popover: "at" },
      { key: "g", ctrlKey: true, code: "KeyG" },
    )
    expect(result).toBe("close-popover")
  })

  test("ctrl+g while working triggers abort", () => {
    const result = keyHandlerDecision(
      { ...baseState, working: true },
      { key: "g", ctrlKey: true, code: "KeyG" },
    )
    expect(result).toBe("abort")
  })

  test("ctrl+u in normal mode triggers file picker", () => {
    const result = keyHandlerDecision(baseState, {
      key: "u",
      ctrlKey: true,
    })
    expect(result).toBe("file-pick")
  })

  test("ctrl+u in shell mode passes through", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell" },
      { key: "u", ctrlKey: true },
    )
    expect(result).toBe("pass-through")
  })

  test("escape popover priority: popover takes precedence over shell mode", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell", popover: "at" },
      { key: "Escape" },
    )
    expect(result).toBe("close-popover")
  })

  test("escape priority: shell mode takes precedence over working", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell", working: true },
      { key: "Escape" },
    )
    expect(result).toBe("normal-mode")
  })

  test("escape priority: popover wins over both shell mode and working", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell", popover: "slash", working: true },
      { key: "Escape" },
    )
    expect(result).toBe("close-popover")
  })
})

// ---------------------------------------------------------------------------
// Tests: popover navigation keys
// ---------------------------------------------------------------------------
describe("popover navigation", () => {
  const baseState: KeyHandlerState = {
    mode: "normal",
    popover: null,
    working: false,
    cursorPosition: 0,
    textLength: 0,
    isImeComposing: false,
  }

  test("ArrowDown in popover triggers popover navigation", () => {
    const result = keyHandlerDecision(
      { ...baseState, popover: "slash" },
      { key: "ArrowDown" },
    )
    expect(result).toBe("popover-nav")
  })

  test("ArrowUp in popover triggers popover navigation", () => {
    const result = keyHandlerDecision(
      { ...baseState, popover: "slash" },
      { key: "ArrowUp" },
    )
    expect(result).toBe("popover-nav")
  })

  test("Tab in popover triggers popover navigation", () => {
    const result = keyHandlerDecision(
      { ...baseState, popover: "at" },
      { key: "Tab" },
    )
    expect(result).toBe("popover-nav")
  })

  test("Enter in popover triggers popover navigation instead of submit", () => {
    const result = keyHandlerDecision(
      { ...baseState, popover: "slash" },
      { key: "Enter" },
    )
    expect(result).toBe("popover-nav")
  })

  test("Ctrl+N in popover triggers popover navigation", () => {
    const result = keyHandlerDecision(
      { ...baseState, popover: "at" },
      { key: "n", ctrlKey: true },
    )
    expect(result).toBe("popover-nav")
  })

  test("Ctrl+P in popover triggers popover navigation", () => {
    const result = keyHandlerDecision(
      { ...baseState, popover: "ask" },
      { key: "p", ctrlKey: true },
    )
    expect(result).toBe("popover-nav")
  })

  test("ArrowDown without popover passes through", () => {
    const result = keyHandlerDecision(baseState, { key: "ArrowDown" })
    expect(result).toBe("pass-through")
  })

  test("ArrowUp without popover passes through", () => {
    const result = keyHandlerDecision(baseState, { key: "ArrowUp" })
    expect(result).toBe("pass-through")
  })

  test("Tab without popover passes through", () => {
    const result = keyHandlerDecision(baseState, { key: "Tab" })
    expect(result).toBe("pass-through")
  })

  test("Ctrl+N without popover passes through", () => {
    const result = keyHandlerDecision(baseState, { key: "n", ctrlKey: true })
    expect(result).toBe("pass-through")
  })
})

// ---------------------------------------------------------------------------
// Tests: shell mode transitions
// ---------------------------------------------------------------------------
describe("shell mode transitions", () => {
  const baseState: KeyHandlerState = {
    mode: "normal",
    popover: null,
    working: false,
    cursorPosition: 0,
    textLength: 0,
    isImeComposing: false,
  }

  test("exclamation at position 0 with existing text still enters shell mode", () => {
    const result = keyHandlerDecision(
      { ...baseState, cursorPosition: 0, textLength: 10 },
      { key: "!" },
    )
    expect(result).toBe("shell-mode")
  })

  test("backspace in shell mode at position 1 passes through", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell", cursorPosition: 1, textLength: 3 },
      { key: "Backspace" },
    )
    expect(result).toBe("pass-through")
  })

  test("backspace in shell mode at position 0 with remaining text passes through", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell", cursorPosition: 0, textLength: 3 },
      { key: "Backspace" },
    )
    expect(result).toBe("pass-through")
  })

  test("enter in shell mode submits the shell command", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell", textLength: 5 },
      { key: "Enter" },
    )
    expect(result).toBe("submit")
  })

  test("shift+enter in shell mode inserts newline", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell" },
      { key: "Enter", shiftKey: true },
    )
    expect(result).toBe("insert-newline")
  })
})

// ---------------------------------------------------------------------------
// Tests: Cmd+U (macOS) file picker
// ---------------------------------------------------------------------------
describe("file picker shortcut", () => {
  const baseState: KeyHandlerState = {
    mode: "normal",
    popover: null,
    working: false,
    cursorPosition: 0,
    textLength: 0,
    isImeComposing: false,
  }

  test("Cmd+U (metaKey) in normal mode triggers file picker", () => {
    const result = keyHandlerDecision(baseState, {
      key: "u",
      metaKey: true,
    })
    expect(result).toBe("file-pick")
  })

  test("Cmd+U in shell mode passes through", () => {
    const result = keyHandlerDecision(
      { ...baseState, mode: "shell" },
      { key: "u", metaKey: true },
    )
    expect(result).toBe("pass-through")
  })

  test("Ctrl+Shift+U does not trigger file picker", () => {
    const result = keyHandlerDecision(baseState, {
      key: "U",
      ctrlKey: true,
      shiftKey: true,
    })
    expect(result).toBe("pass-through")
  })

  test("Alt+U does not trigger file picker", () => {
    const result = keyHandlerDecision(baseState, {
      key: "u",
      altKey: true,
    })
    expect(result).toBe("pass-through")
  })
})

// ---------------------------------------------------------------------------
// Tests: Ctrl+G behavior
// ---------------------------------------------------------------------------
describe("ctrl+g behavior", () => {
  const baseState: KeyHandlerState = {
    mode: "normal",
    popover: null,
    working: false,
    cursorPosition: 0,
    textLength: 0,
    isImeComposing: false,
  }

  test("ctrl+g with no popover and not working passes through", () => {
    const result = keyHandlerDecision(baseState, {
      key: "g",
      ctrlKey: true,
      code: "KeyG",
    })
    expect(result).toBe("pass-through")
  })

  test("ctrl+g prioritizes popover over working", () => {
    const result = keyHandlerDecision(
      { ...baseState, popover: "slash", working: true },
      { key: "g", ctrlKey: true, code: "KeyG" },
    )
    expect(result).toBe("close-popover")
  })
})

// ---------------------------------------------------------------------------
// Tests: pass-through for unhandled keys
// ---------------------------------------------------------------------------
describe("unhandled keys pass through", () => {
  const baseState: KeyHandlerState = {
    mode: "normal",
    popover: null,
    working: false,
    cursorPosition: 0,
    textLength: 0,
    isImeComposing: false,
  }

  test("regular letter key passes through", () => {
    const result = keyHandlerDecision(baseState, { key: "a" })
    expect(result).toBe("pass-through")
  })

  test("space key passes through", () => {
    const result = keyHandlerDecision(baseState, { key: " " })
    expect(result).toBe("pass-through")
  })

  test("delete key passes through in normal mode", () => {
    const result = keyHandlerDecision(baseState, { key: "Delete" })
    expect(result).toBe("pass-through")
  })
})

// ---------------------------------------------------------------------------
// Tests: autocomplete trigger patterns (from handleInput)
// ---------------------------------------------------------------------------
describe("autocomplete trigger patterns", () => {
  describe("slash command trigger", () => {
    test("slash alone at position 0 triggers autocomplete", () => {
      expect("/".match(slashPattern)).not.toBeNull()
    })

    test("slash followed by command name triggers autocomplete", () => {
      const match = "/help".match(slashPattern)
      expect(match).not.toBeNull()
      expect(match![1]).toBe("help")
    })

    test("slash mid-text does not trigger autocomplete", () => {
      expect("hello /help".match(slashPattern)).toBeNull()
    })

    test("slash with space after does not trigger autocomplete", () => {
      expect("/ help".match(slashPattern)).toBeNull()
    })

    test("slash followed by multi-word does not trigger autocomplete", () => {
      expect("/help me".match(slashPattern)).toBeNull()
    })

    test("empty string does not trigger slash autocomplete", () => {
      expect("".match(slashPattern)).toBeNull()
    })
  })

  describe("at-mention trigger", () => {
    test("at sign triggers mention autocomplete", () => {
      const text = "@"
      const match = text.substring(0, text.length).match(atPattern)
      expect(match).not.toBeNull()
      expect(match![1]).toBe("")
    })

    test("at sign with partial name triggers mention autocomplete", () => {
      const text = "hello @user"
      const match = text.substring(0, text.length).match(atPattern)
      expect(match).not.toBeNull()
      expect(match![1]).toBe("user")
    })

    test("at sign mid-sentence triggers autocomplete", () => {
      const text = "check @file"
      const match = text.substring(0, text.length).match(atPattern)
      expect(match).not.toBeNull()
      expect(match![1]).toBe("file")
    })

    test("at sign with space after does not match a query", () => {
      const text = "@ something"
      const cursorAfterAt = 1
      const match = text.substring(0, cursorAfterAt).match(atPattern)
      expect(match).not.toBeNull()
      expect(match![1]).toBe("")
    })

    test("no at sign does not trigger mention autocomplete", () => {
      const text = "hello world"
      expect(text.match(atPattern)).toBeNull()
    })
  })

  describe("ask command trigger", () => {
    test("ask command with partial agent triggers autocomplete", () => {
      const match = "/ask claude".match(askPattern)
      expect(match).not.toBeNull()
      expect(match![1]).toBe("claude")
    })

    test("ask command without agent does not trigger", () => {
      expect("/ask".match(askPattern)).toBeNull()
    })

    test("ask command with space but no agent captures empty query", () => {
      // \S* matches zero or more non-whitespace, so /ask + space matches with empty capture
      const match = "/ask ".match(askPattern)
      expect(match).not.toBeNull()
      expect(match![1]).toBe("")
    })
  })
})
