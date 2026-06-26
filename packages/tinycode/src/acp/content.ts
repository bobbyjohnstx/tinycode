import type { ContentBlock, ContentChunk } from "@agentclientprotocol/sdk"
import type { TextPartInput, FilePartInput } from "@tinycode/sdk"

export type PromptPart = TextPartInput | FilePartInput

export type ReplayPart =
  | {
      type: "text"
      text: string
      synthetic?: boolean
      ignored?: boolean
    }
  | {
      type: "file"
      url: string
      mime: string
      filename?: string
    }
  | {
      type: "reasoning"
      text: string
    }

export function promptContentToParts(content: readonly ContentBlock[]): PromptPart[] {
  return content.flatMap(contentBlockToParts)
}

export function contentBlockToParts(block: ContentBlock): PromptPart[] {
  switch (block.type) {
    case "text":
      return [
        {
          type: "text",
          text: block.text,
        },
      ]

    case "image":
      if (block.data) {
        return [
          {
            type: "file",
            url: `data:${block.mimeType};base64,${block.data}`,
            filename: block.uri ? filenameFromUri(block.uri) : "image",
            mime: block.mimeType,
          },
        ]
      }
      if (block.uri?.startsWith("data:")) {
        return [
          {
            type: "file",
            url: block.uri,
            filename: "image",
            mime: block.mimeType,
          },
        ]
      }
      if (block.uri?.startsWith("http://") || block.uri?.startsWith("https://")) {
        return [
          {
            type: "file",
            url: block.uri,
            filename: filenameFromUri(block.uri) ?? "image",
            mime: block.mimeType,
          },
        ]
      }
      return []

    case "resource_link":
      return [
        {
          type: "text",
          text: `[${block.uri}]`,
        },
      ]

    case "resource":
      if ("text" in block.resource) {
        return [{ type: "text", text: `[${block.resource.uri}]\n${block.resource.text}` }]
      }
      if (block.resource.mimeType) {
        return [
          {
            type: "file",
            url: block.resource.uri.startsWith("data:")
              ? block.resource.uri
              : `data:${block.resource.mimeType};base64,${block.resource.blob}`,
            filename: filenameFromUri(block.resource.uri) ?? "file",
            mime: block.resource.mimeType,
          },
        ]
      }
      return []

    default:
      return []
  }
}

export function partsToContentChunks(parts: readonly ReplayPart[]): ContentChunk[] {
  return parts.flatMap(partToContentChunks)
}

export function partToContentChunks(part: ReplayPart): ContentChunk[] {
  switch (part.type) {
    case "text":
      if (!part.text) return []
      return [
        {
          content: {
            type: "text",
            text: part.text,
          },
        },
      ]

    case "file":
      return [
        {
          content: {
            type: "image",
            mimeType: part.mime,
            data: extractBase64(part.url) ?? "",
            uri: part.url,
          },
        },
      ]

    case "reasoning":
      if (!part.text) return []
      return [
        {
          content: {
            type: "text",
            text: part.text,
          },
        },
      ]
  }
}

function filenameFromUri(uri: string): string | undefined {
  try {
    const url = new URL(uri)
    const parts = url.pathname.split("/")
    return parts[parts.length - 1] || undefined
  } catch {
    const parts = uri.split("/")
    return parts[parts.length - 1] || undefined
  }
}

function extractBase64(dataUrl: string): string | undefined {
  if (!dataUrl.startsWith("data:")) return undefined
  const parts = dataUrl.split(",")
  return parts[1]
}

export * as ACPContent from "./content"
