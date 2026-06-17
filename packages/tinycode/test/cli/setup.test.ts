import { describe, expect, test } from "bun:test"
import { SetupCommand } from "../../src/cli/cmd/setup"

describe("cli.setup", () => {
  test("SetupCommand has correct command name", () => {
    expect(SetupCommand.command).toBe("setup")
  })

  test("SetupCommand has non-interactive option", () => {
    // Verify the builder adds the expected options by inspecting the yargs builder
    const mockYargs = {
      option: function (name: string, _opts: unknown) {
        this.options.push(name)
        return this
      },
      options: [] as string[],
    }
    SetupCommand.builder(mockYargs as any)
    expect(mockYargs.options).toContain("force")
    expect(mockYargs.options).toContain("non-interactive")
    expect(mockYargs.options).toContain("provider")
    expect(mockYargs.options).toContain("api-key")
  })
})
