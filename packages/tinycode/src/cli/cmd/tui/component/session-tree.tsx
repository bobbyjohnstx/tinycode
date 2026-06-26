import { createMemo, For } from "solid-js"
import { useTheme } from "../context/theme"
import { useSync } from "../context/sync"
import type { Session } from "@tinycode/sdk/v2"
import type { JSX } from "@opentui/solid"

interface TreeNode {
  session: Session
  children: TreeNode[]
  depth: number
  isLast: boolean
  parentChain: string[]
}

interface SessionTreeProps {
  sessionID: string
  onSwitch?: (sessionID: string) => void
}

export function SessionTree(props: SessionTreeProps) {
  const { theme } = useTheme()
  const sync = useSync()

  const currentSession = createMemo(() => sync.session.get(props.sessionID))

  const rootSession = createMemo(() => {
    const current = currentSession()
    if (!current) return undefined
    if (!current.parentID) return current
    return sync.session.get(current.parentID) ?? current
  })

  const tree = createMemo(() => {
    const root = rootSession()
    if (!root) return []

    const buildTree = (sessionID: string, depth: number, parentChain: string[]): TreeNode | null => {
      const session = sync.session.get(sessionID)
      if (!session) return null

      const childrenSessions = sync.data.session.filter((s: Session) => s.parentID === sessionID)
      const childNodes = childrenSessions
        .map((child: Session, _index: number) => buildTree(child.id, depth + 1, [...parentChain, sessionID]))
        .filter((node): node is TreeNode => node !== null)
        .map((node: TreeNode, index: number, arr: TreeNode[]) => ({ ...node, isLast: index === arr.length - 1 }))

      return {
        session,
        children: childNodes,
        depth,
        isLast: true,
        parentChain,
      }
    }

    const rootNode = buildTree(root.id, 0, [])
    return rootNode ? [rootNode] : []
  })

  const flattenedTree = createMemo((): TreeNode[] => {
    const flatten = (nodes: TreeNode[]): TreeNode[] => {
      const result: TreeNode[] = []
      for (const node of nodes) {
        result.push(node)
        // Show all children (fully expanded view)
        if (node.children.length > 0) {
          result.push(...flatten(node.children))
        }
      }
      return result
    }
    return flatten(tree())
  })

  const visibleNodes = createMemo(() => flattenedTree())

  // Future enhancement: keyboard navigation within the tree
  // For now, tree is view-only and session switching happens via click/sidebar toggle

  const getTreePrefix = (node: TreeNode): string => {
    let prefix = ""
    for (let i = 0; i < node.depth; i++) {
      if (i === node.depth - 1) {
        prefix += node.isLast ? "`-- " : "|-- "
      } else {
        const parentAtDepth = node.parentChain[i]
        const parentNode = visibleNodes().find((n: TreeNode) => n.session.id === parentAtDepth)
        prefix += parentNode && !parentNode.isLast ? "|   " : "    "
      }
    }
    return prefix
  }

  const truncate = (text: string, maxLength: number): string => {
    if (text.length <= maxLength) return text
    return text.slice(0, maxLength - 3) + "..."
  }

  return (
    <box flexDirection="column" gap={1}>
      <text fg={theme.textMuted}>Session Tree</text>
      <For each={visibleNodes()}>
        {(node: TreeNode): JSX.Element => {
          const isCurrent = (): boolean => node.session.id === props.sessionID
          const prefix = getTreePrefix(node)
          const title = truncate(node.session.title, 32 - prefix.length)

          return (
            <text fg={isCurrent() ? theme.text : theme.textMuted}>
              {isCurrent() ? "> " : "  "}
              {prefix}
              {title}
            </text>
          ) as JSX.Element
        }}
      </For>
    </box>
  ) as JSX.Element
}
