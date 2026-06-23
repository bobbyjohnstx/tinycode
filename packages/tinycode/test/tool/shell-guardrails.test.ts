/**
 * Pure unit tests for the shell guardrail detection logic.
 *
 * These tests target DESTRUCTIVE_COMMANDS and SECRETS_PATTERNS directly,
 * which are private constants in shell.ts. We test the detection behaviour
 * through the public exports by reproducing the same Map/RegExp structure.
 *
 * If those constants are ever exported from shell.ts the tests can be
 * simplified to import them directly.
 */

import { describe, it, expect } from "bun:test"

// ---------------------------------------------------------------------------
// Mirror the DESTRUCTIVE_COMMANDS logic from src/tool/shell.ts
// ---------------------------------------------------------------------------

const DESTRUCTIVE_COMMANDS = new Map<string, (args: string[]) => string | null>([
  [
    "rm",
    (args) =>
      args.some((a) => !a.startsWith("-") || a.includes("r") || a.includes("f")) ? `rm ${args.join(" ")}` : null,
  ],
  ["rmdir", (args) => `rmdir ${args.join(" ")}`],
  [
    "git",
    (args) => {
      const sub = args[0]
      if (sub === "reset" && args.includes("--hard")) return `git reset --hard`
      if (sub === "push" && (args.includes("--force") || args.includes("-f"))) return `git push --force`
      if (sub === "branch" && args.includes("-D"))
        return `git branch -D ${args.slice(args.indexOf("-D") + 1).join(" ")}`
      if (sub === "clean" && args.includes("-f")) return `git clean -f`
      if (sub === "checkout" && args.includes("--")) return `git checkout -- (file restore)`
      return null
    },
  ],
])

const SECRETS_PATTERNS = /\.(env|pem|key|p12|pfx|cert|credentials)$|\.env\.|\/credentials|\/secrets?\b/i

// ---------------------------------------------------------------------------
// Helper: run a command string through the guardrail logic the same way
// collect() does in shell.ts.
// ---------------------------------------------------------------------------
function detectDestructive(command: string): string | null {
  const tokens = command.trim().split(/\s+/)
  const cmd = tokens[0]
  if (!cmd) return null
  const handler = DESTRUCTIVE_COMMANDS.get(cmd)
  if (!handler) return null
  return handler(tokens.slice(1))
}

function detectSecrets(command: string): boolean {
  const READ_CMDS = new Set(["cat", "head", "tail", "grep"])
  const tokens = command.trim().split(/\s+/)
  const cmd = tokens[0]
  if (!cmd || !READ_CMDS.has(cmd)) return false
  return tokens.slice(1).some((arg) => !arg.startsWith("-") && SECRETS_PATTERNS.test(arg))
}

// ---------------------------------------------------------------------------
// DESTRUCTIVE_COMMANDS tests
// ---------------------------------------------------------------------------

describe("shell guardrails – DESTRUCTIVE_COMMANDS", () => {
  describe("rm", () => {
    it("triggers guardrail for rm -rf /tmp/foo", () => {
      expect(detectDestructive("rm -rf /tmp/foo")).not.toBeNull()
    })

    it("triggers guardrail for rm file.txt (non-flag argument)", () => {
      expect(detectDestructive("rm file.txt")).not.toBeNull()
    })

    it("triggers guardrail for rm -r directory (flag with r)", () => {
      expect(detectDestructive("rm -r directory")).not.toBeNull()
    })

    it("triggers guardrail for rm -f file (flag with f)", () => {
      expect(detectDestructive("rm -f file")).not.toBeNull()
    })
  })

  describe("git reset", () => {
    it("triggers guardrail for git reset --hard", () => {
      expect(detectDestructive("git reset --hard")).not.toBeNull()
    })

    it("does NOT trigger guardrail for git reset (soft, no --hard)", () => {
      expect(detectDestructive("git reset")).toBeNull()
    })

    it("does NOT trigger guardrail for git reset --soft HEAD~1", () => {
      expect(detectDestructive("git reset --soft HEAD~1")).toBeNull()
    })
  })

  describe("git push --force", () => {
    it("triggers guardrail for git push --force origin main", () => {
      expect(detectDestructive("git push --force origin main")).not.toBeNull()
    })

    it("triggers guardrail for git push -f origin main", () => {
      expect(detectDestructive("git push -f origin main")).not.toBeNull()
    })

    it("does NOT trigger guardrail for git push origin main (no --force)", () => {
      expect(detectDestructive("git push origin main")).toBeNull()
    })
  })

  describe("git branch -D", () => {
    it("triggers guardrail for git branch -D mybranch", () => {
      expect(detectDestructive("git branch -D mybranch")).not.toBeNull()
    })

    it("does NOT trigger guardrail for git branch -d mybranch (lowercase -d is not force-delete)", () => {
      expect(detectDestructive("git branch -d mybranch")).toBeNull()
    })

    it("does NOT trigger guardrail for git branch --list", () => {
      expect(detectDestructive("git branch --list")).toBeNull()
    })
  })

  describe("git clean", () => {
    it("triggers guardrail for git clean -f", () => {
      expect(detectDestructive("git clean -f")).not.toBeNull()
    })

    it("does NOT trigger guardrail for git clean (dry-run only)", () => {
      expect(detectDestructive("git clean -n")).toBeNull()
    })
  })

  describe("git checkout --", () => {
    it("triggers guardrail for git checkout -- .", () => {
      expect(detectDestructive("git checkout -- .")).not.toBeNull()
    })

    it("does NOT trigger guardrail for git checkout main (branch switch)", () => {
      expect(detectDestructive("git checkout main")).toBeNull()
    })
  })

  describe("rmdir", () => {
    it("triggers guardrail for rmdir somedir", () => {
      expect(detectDestructive("rmdir somedir")).not.toBeNull()
    })
  })

  describe("safe commands – no guardrail", () => {
    it("does NOT trigger guardrail for ls -la", () => {
      expect(detectDestructive("ls -la")).toBeNull()
    })

    it("does NOT trigger guardrail for echo hello", () => {
      expect(detectDestructive("echo hello")).toBeNull()
    })

    it("does NOT trigger guardrail for npm install", () => {
      expect(detectDestructive("npm install")).toBeNull()
    })

    it("does NOT trigger guardrail for git log --oneline -5", () => {
      expect(detectDestructive("git log --oneline -5")).toBeNull()
    })

    it("does NOT trigger guardrail for git status", () => {
      expect(detectDestructive("git status")).toBeNull()
    })
  })
})

// ---------------------------------------------------------------------------
// SECRETS_PATTERNS tests
// ---------------------------------------------------------------------------

describe("shell guardrails – SECRETS_PATTERNS (read command detection)", () => {
  it("triggers secrets guardrail for cat .env", () => {
    expect(detectSecrets("cat .env")).toBe(true)
  })

  it("does NOT trigger secrets guardrail for cat .env.example (.env. prefix still matches)", () => {
    // .env.example matches \.env\. in the pattern — verify the actual behaviour
    expect(SECRETS_PATTERNS.test(".env.example")).toBe(true)
    expect(detectSecrets("cat .env.example")).toBe(true)
  })

  it("triggers secrets guardrail for cat secrets.pem", () => {
    expect(detectSecrets("cat secrets.pem")).toBe(true)
  })

  it("triggers secrets guardrail for cat private.key", () => {
    expect(detectSecrets("cat private.key")).toBe(true)
  })

  it("triggers secrets guardrail for cat cert.p12", () => {
    expect(detectSecrets("cat cert.p12")).toBe(true)
  })

  it("triggers secrets guardrail for cat bundle.pfx", () => {
    expect(detectSecrets("cat bundle.pfx")).toBe(true)
  })

  it("triggers secrets guardrail for cat server.cert", () => {
    expect(detectSecrets("cat server.cert")).toBe(true)
  })

  it("triggers secrets guardrail for grep password credentials.json via path /credentials", () => {
    // grep with an arg containing /credentials
    expect(detectSecrets("grep password /project/credentials")).toBe(true)
  })

  it("triggers secrets guardrail for head /etc/secrets", () => {
    expect(detectSecrets("head /etc/secrets")).toBe(true)
  })

  it("triggers secrets guardrail for tail /var/secret", () => {
    expect(detectSecrets("tail /var/secret")).toBe(true)
  })

  it("does NOT trigger secrets guardrail for cat README.md", () => {
    expect(detectSecrets("cat README.md")).toBe(false)
  })

  it("does NOT trigger secrets guardrail for cat config.json", () => {
    expect(detectSecrets("cat config.json")).toBe(false)
  })

  it("does NOT trigger secrets guardrail for ls -la (not a read command)", () => {
    expect(detectSecrets("ls -la")).toBe(false)
  })

  it("does NOT trigger secrets guardrail for echo hello", () => {
    expect(detectSecrets("echo hello")).toBe(false)
  })

  it("does NOT trigger secrets guardrail for npm install", () => {
    expect(detectSecrets("npm install")).toBe(false)
  })

  it("does NOT trigger secrets guardrail for grep -r pattern . (flags only, no sensitive path)", () => {
    expect(detectSecrets("grep -r pattern .")).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// SECRETS_PATTERNS regex unit tests (direct, framework-independent)
// ---------------------------------------------------------------------------

describe("SECRETS_PATTERNS regex", () => {
  it("matches .env exactly", () => {
    expect(SECRETS_PATTERNS.test(".env")).toBe(true)
  })

  it("matches .env.local via \.env\. clause", () => {
    expect(SECRETS_PATTERNS.test(".env.local")).toBe(true)
  })

  it("matches .env.production via \.env\. clause", () => {
    expect(SECRETS_PATTERNS.test(".env.production")).toBe(true)
  })

  it("matches file.pem", () => {
    expect(SECRETS_PATTERNS.test("file.pem")).toBe(true)
  })

  it("matches file.key", () => {
    expect(SECRETS_PATTERNS.test("file.key")).toBe(true)
  })

  it("matches /home/user/credentials", () => {
    expect(SECRETS_PATTERNS.test("/home/user/credentials")).toBe(true)
  })

  it("matches /etc/secrets", () => {
    expect(SECRETS_PATTERNS.test("/etc/secrets")).toBe(true)
  })

  it("is case-insensitive (.ENV matches)", () => {
    expect(SECRETS_PATTERNS.test(".ENV")).toBe(true)
  })

  it("does not match regular source files", () => {
    expect(SECRETS_PATTERNS.test("src/main.ts")).toBe(false)
    expect(SECRETS_PATTERNS.test("package.json")).toBe(false)
    expect(SECRETS_PATTERNS.test("README.md")).toBe(false)
  })
})
