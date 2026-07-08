import { describe, test, expect } from "bun:test"
import { readFileSync, statSync } from "fs"
import { execSync } from "child_process"
import path from "path"

const INSTALL_SCRIPT = path.join(import.meta.dir, "../../../../install.sh")

describe("install.sh", () => {
  test("exists and is executable", () => {
    const stat = statSync(INSTALL_SCRIPT)
    expect(stat.mode & 0o111).toBeGreaterThan(0)
  })

  test("has correct shebang", () => {
    const content = readFileSync(INSTALL_SCRIPT, "utf-8")
    expect(content.startsWith("#!/usr/bin/env sh") || content.startsWith("#!/bin/sh")).toBe(true)
  })

  test("passes syntax check", () => {
    expect(() => execSync(`sh -n ${INSTALL_SCRIPT}`)).not.toThrow()
  })

  test("contains platform detection for all supported targets", () => {
    const content = readFileSync(INSTALL_SCRIPT, "utf-8")
    expect(content).toContain("darwin")
    expect(content).toContain("linux")
    expect(content).toContain("arm64")
    expect(content).toContain("x86_64")
  })

  test("downloads from correct GitHub repo", () => {
    const content = readFileSync(INSTALL_SCRIPT, "utf-8")
    expect(content).toContain("bobbyjohnstx/tinycode")
  })

  test("defaults to ~/.local/bin install directory", () => {
    const content = readFileSync(INSTALL_SCRIPT, "utf-8")
    expect(content).toContain(".local/bin")
  })
})
