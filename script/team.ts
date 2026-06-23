#!/usr/bin/env bun
// Wrapper around oh-my-tiny team coordinator with tinycode defaults.
// Usage: bun script/team.ts --team <name> --workers <n> --task "<desc>"
import { $ } from "bun"
import path from "path"

const COORDINATOR = path.join(import.meta.dirname, "../node_modules/oh-my-tiny/dist/team/coordinator.js")
const SERVER_URL = process.env.TINYCODE_SERVER_URL ?? "http://localhost:4096"
const CWD =
  (process.argv.includes("--cwd") ? process.argv[process.argv.indexOf("--cwd") + 1] : undefined) ?? process.cwd()

// Pass all args through, injecting --server-url and --cwd defaults
const args = process.argv.slice(2)
if (!args.includes("--server-url")) args.push("--server-url", SERVER_URL)
if (!args.includes("--cwd")) args.push("--cwd", CWD)

await $`node ${COORDINATOR} ${args}`
