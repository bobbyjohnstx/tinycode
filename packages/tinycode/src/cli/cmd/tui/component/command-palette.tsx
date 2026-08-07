import { createMemo, createResource } from "solid-js"
import { DialogSelect, type DialogSelectRef } from "@tui/ui/dialog-select"
import { type DialogContext } from "@tui/ui/dialog"
import {
  COMMAND_PALETTE_COMMAND,
  formatKeyBindings,
  type OpenTuiKeymap,
  useKeymapSelector,
  useTinycodeKeymap,
} from "../keymap"
import { useTuiConfig } from "../context/tui-config"
import { useFrecency } from "./prompt/frecency"
import { useLocal } from "@tui/context/local"
import { useSync } from "@tui/context/sync"
import { useRoute } from "@tui/context/route"
import { useSDK } from "@tui/context/sdk"
import { Locale } from "@/util/locale"

type PaletteCommandEntry = ReturnType<OpenTuiKeymap["getCommandEntries"]>[number]

function isVisiblePaletteCommand(command: PaletteCommandEntry["command"]) {
  return command.hidden !== true && command.name !== COMMAND_PALETTE_COMMAND
}

function isSuggestedPaletteCommand(entry: PaletteCommandEntry) {
  const suggested = entry.command.suggested
  if (typeof suggested === "boolean") return suggested
  if (typeof suggested === "function") return suggested() === true
  return false
}

export function CommandPaletteDialog() {
  const config = useTuiConfig()
  const keymap = useTinycodeKeymap()
  const frecency = useFrecency()
  const local = useLocal()
  const sync = useSync()
  const route = useRoute()
  const sdk = useSDK()

  const entries = useKeymapSelector((keymap: OpenTuiKeymap) => {
    const query = {
      namespace: "palette",
    }
    const reachable = keymap.getCommandEntries({
      ...query,
      visibility: "reachable",
      filter: isVisiblePaletteCommand,
    })
    const registeredBindings = keymap.getCommandBindings({
      visibility: "registered",
      commands: reachable.map((entry) => entry.command.name),
    })

    return reachable.map((entry) => ({
      ...entry,
      bindings: registeredBindings.get(entry.command.name) ?? entry.bindings,
    }))
  })

  const commandOptions = createMemo(() =>
    entries().map((entry) => ({
      title: typeof entry.command.title === "string" ? entry.command.title : entry.command.name,
      description: typeof entry.command.desc === "string" ? entry.command.desc : undefined,
      category: typeof entry.command.category === "string" ? entry.command.category : "Commands",
      footer: formatKeyBindings(entry.bindings, config),
      value: entry.command.name,
      suggested: isSuggestedPaletteCommand(entry),
      frecencyScore: frecency.getFrecency("commands", entry.command.name),
      onSelect: (dialog: DialogContext) => {
        dialog.clear()
        frecency.updateFrecency("commands", entry.command.name)
        keymap.dispatchCommand(entry.command.name)
      },
    })),
  )

  const currentAgent = createMemo(() => local.agent.current()?.name)
  const agentOptions = createMemo(() =>
    local.agent
      .list()
      .filter((a) => !a.hidden && a.name !== currentAgent())
      .map((agent) => ({
        title: agent.name,
        description: agent.description ? Locale.truncate(agent.description, 60) : undefined,
        category: "Agents",
        footer: agent.name === currentAgent() ? "current" : "",
        value: `agent:${agent.name}`,
        suggested: false,
        frecencyScore: frecency.getFrecency("agents", agent.name),
        onSelect: (dialog: DialogContext) => {
          dialog.clear()
          frecency.updateFrecency("agents", agent.name)
          local.agent.set(agent.name)
        },
      })),
  )

  const currentSessionID = createMemo(() => (route.data.type === "session" ? route.data.sessionID : undefined))
  const sessionOptions = createMemo(() =>
    sync.data.session
      .filter((s) => !s.parentID && s.id !== currentSessionID())
      .toSorted((a, b) => b.time.updated - a.time.updated)
      .slice(0, 8)
      .map((session) => ({
        title: session.title,
        description: Locale.time(session.time.updated),
        category: "Sessions",
        footer: "",
        value: `session:${session.id}`,
        suggested: false,
        frecencyScore: frecency.getFrecency("sessions", session.id),
        onSelect: (dialog: DialogContext) => {
          dialog.clear()
          frecency.updateFrecency("sessions", session.id)
          route.navigate({ type: "session", sessionID: session.id })
        },
      })),
  )

  const [skills] = createResource(async () => {
    const result = await sdk.client.app.skills()
    return result.data ?? []
  })
  const skillOptions = createMemo(() =>
    (skills() ?? []).map((skill) => ({
      title: `/${skill.name}`,
      description: skill.description?.replace(/\s+/g, " ").trim(),
      category: "Skills",
      footer: "",
      value: `skill:${skill.name}`,
      suggested: false,
      frecencyScore: frecency.getFrecency("skills", skill.name),
      onSelect: (dialog: DialogContext) => {
        dialog.clear()
        frecency.updateFrecency("skills", skill.name)
        keymap.dispatchCommand("prompt.skills")
      },
    })),
  )

  let ref: DialogSelectRef<string>
  const list = () => {
    if (ref?.filter) {
      return [...commandOptions(), ...agentOptions(), ...sessionOptions(), ...skillOptions()]
    }
    const suggested = commandOptions()
      .filter((option) => option.suggested)
      .map((option) => ({
        ...option,
        value: `suggested:${option.value}`,
        category: "Suggested",
      }))
    const commands = [...commandOptions()].sort((a, b) => b.frecencyScore - a.frecencyScore)
    const agents = [...agentOptions()].sort((a, b) => b.frecencyScore - a.frecencyScore)
    const sessions = [...sessionOptions()]
    const skillsList = [...skillOptions()].sort((a, b) => b.frecencyScore - a.frecencyScore)
    return [...suggested, ...commands, ...agents, ...sessions, ...skillsList]
  }

  return <DialogSelect ref={(value) => (ref = value)} title="Command Palette" options={list()} flat />
}
