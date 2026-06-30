import { describe, expect, test } from "bun:test"
import { Wildcard } from "@/util/wildcard"

describe("Wildcard.match", () => {
  test("matches exact strings", () => {
    expect(Wildcard.match("hello", "hello")).toBe(true)
    expect(Wildcard.match("hello", "world")).toBe(false)
  })

  test("matches * wildcard", () => {
    expect(Wildcard.match("hello.ts", "*.ts")).toBe(true)
    expect(Wildcard.match("hello.js", "*.ts")).toBe(false)
    expect(Wildcard.match("src/hello.ts", "src/*.ts")).toBe(true)
    expect(Wildcard.match("src/util/test.ts", "src/*/test.ts")).toBe(true)
  })

  test("matches multiple levels with wildcard", () => {
    expect(Wildcard.match("src/util/test.ts", "*/*/*.ts")).toBe(true)
    expect(Wildcard.match("a/b.ts", "*/*.ts")).toBe(true)
  })

  test("matches ? wildcard", () => {
    expect(Wildcard.match("test1.ts", "test?.ts")).toBe(true)
    expect(Wildcard.match("test12.ts", "test?.ts")).toBe(false)
    expect(Wildcard.match("test.ts", "test?.ts")).toBe(false)
  })

  test("normalizes backslashes to forward slashes", () => {
    expect(Wildcard.match("src\\util\\test.ts", "src/util/test.ts")).toBe(true)
    expect(Wildcard.match("src/util/test.ts", "src\\util\\test.ts")).toBe(true)
  })

  test("handles trailing space wildcard as optional", () => {
    expect(Wildcard.match("ls", "ls *")).toBe(true)
    expect(Wildcard.match("ls -la", "ls *")).toBe(true)
    expect(Wildcard.match("ls -la -h", "ls *")).toBe(true)
    expect(Wildcard.match("cd", "ls *")).toBe(false)
  })

  test("escapes special regex characters", () => {
    expect(Wildcard.match("test.file", "test.file")).toBe(true)
    expect(Wildcard.match("testXfile", "test.file")).toBe(false)
    expect(Wildcard.match("test[1].ts", "test[1].ts")).toBe(true)
    expect(Wildcard.match("test+plus.ts", "test+plus.ts")).toBe(true)
  })

  test("is case sensitive on non-Windows platforms", () => {
    if (process.platform !== "win32") {
      expect(Wildcard.match("Hello", "hello")).toBe(false)
      expect(Wildcard.match("TEST.TS", "test.ts")).toBe(false)
    }
  })
})

describe("Wildcard.all", () => {
  test("returns value for matching pattern", () => {
    const patterns = {
      "*.ts": "typescript",
      "*.js": "javascript",
    }
    expect(Wildcard.all("test.ts", patterns)).toBe("typescript")
    expect(Wildcard.all("test.js", patterns)).toBe("javascript")
    expect(Wildcard.all("test.py", patterns)).toBeUndefined()
  })

  test("prefers longest matching pattern", () => {
    const patterns = {
      "*": "any",
      "*.ts": "typescript",
      "src/*.ts": "source-ts",
    }
    expect(Wildcard.all("test.ts", patterns)).toBe("typescript")
    expect(Wildcard.all("src/test.ts", patterns)).toBe("source-ts")
    expect(Wildcard.all("test.py", patterns)).toBe("any")
  })

  test("returns last match when multiple patterns of same length match", () => {
    const patterns = {
      "*.ts": "first",
      "test.*": "second",
    }
    expect(Wildcard.all("test.ts", patterns)).toBe("second")
  })

  test("returns undefined when no pattern matches", () => {
    const patterns = {
      "*.ts": "typescript",
    }
    expect(Wildcard.all("test.js", patterns)).toBeUndefined()
  })
})

describe("Wildcard.allStructured", () => {
  test("matches head and tail patterns", () => {
    const patterns = {
      "git commit *": "commit",
      "git push *": "push",
    }
    expect(Wildcard.allStructured({ head: "git", tail: ["commit", "-m", "test"] }, patterns)).toBe("commit")
    expect(Wildcard.allStructured({ head: "git", tail: ["push", "origin", "main"] }, patterns)).toBe("push")
  })

  test("matches exact command without tail", () => {
    const patterns = {
      git: "git-tool",
      npm: "npm-tool",
    }
    expect(Wildcard.allStructured({ head: "git", tail: [] }, patterns)).toBe("git-tool")
    expect(Wildcard.allStructured({ head: "npm", tail: [] }, patterns)).toBe("npm-tool")
  })

  test("matches patterns with multiple parts", () => {
    const patterns = {
      "git commit": "commit-no-args",
      "git push": "push-cmd",
    }
    expect(Wildcard.allStructured({ head: "git", tail: ["commit"] }, patterns)).toBe("commit-no-args")
    expect(Wildcard.allStructured({ head: "git", tail: ["push"] }, patterns)).toBe("push-cmd")
  })

  test("returns undefined when no pattern matches", () => {
    const patterns = {
      "git *": "git-cmd",
    }
    expect(Wildcard.allStructured({ head: "npm", tail: ["install"] }, patterns)).toBeUndefined()
  })

  test("prefers longer patterns", () => {
    const patterns = {
      "git *": "any-git",
      "git commit *": "commit",
    }
    expect(Wildcard.allStructured({ head: "git", tail: ["commit", "-m", "test"] }, patterns)).toBe("commit")
    expect(Wildcard.allStructured({ head: "git", tail: ["push"] }, patterns)).toBe("any-git")
  })
})
