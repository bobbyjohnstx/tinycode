import { execSync } from "child_process"
import { writeFileSync, unlinkSync } from "fs"
import { tmpdir } from "os"
import { join } from "path"

const startsWith = (bytes: Uint8Array, prefix: number[]) => prefix.every((value, index) => bytes[index] === value)

/** Extract text from a PDF file using pdftotext (poppler). Returns null if unavailable or extraction fails. */
export function pdfFileToText(filepath: string): string | null {
  try {
    const out = execSync(`pdftotext "${filepath.replace(/"/g, '\\"')}" -`, {
      encoding: "utf-8",
      timeout: 30_000,
      stdio: ["pipe", "pipe", "pipe"],
    }).trim()
    return out.length > 0 ? out : null
  } catch {
    return null
  }
}

/** Extract text from PDF bytes by writing a temp file. Returns null if unavailable or extraction fails. */
export function pdfBytesToText(bytes: Uint8Array): string | null {
  const tmp = join(tmpdir(), `tc-pdf-${Date.now()}.pdf`)
  try {
    writeFileSync(tmp, bytes)
    return pdfFileToText(tmp)
  } finally {
    try {
      unlinkSync(tmp)
    } catch {
      /* ignore */
    }
  }
}

export function isPdfAttachment(mime: string) {
  return mime === "application/pdf"
}

export function isMedia(mime: string) {
  return mime.startsWith("image/") || isPdfAttachment(mime)
}

export function isImageAttachment(mime: string) {
  return mime.startsWith("image/") && mime !== "image/svg+xml" && mime !== "image/vnd.fastbidsheet"
}

export function sniffAttachmentMime(bytes: Uint8Array, fallback: string) {
  if (startsWith(bytes, [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])) return "image/png"
  if (startsWith(bytes, [0xff, 0xd8, 0xff])) return "image/jpeg"
  if (startsWith(bytes, [0x47, 0x49, 0x46, 0x38])) return "image/gif"
  if (startsWith(bytes, [0x42, 0x4d])) return "image/bmp"
  if (startsWith(bytes, [0x25, 0x50, 0x44, 0x46, 0x2d])) return "application/pdf"
  if (startsWith(bytes, [0x52, 0x49, 0x46, 0x46]) && startsWith(bytes.subarray(8), [0x57, 0x45, 0x42, 0x50])) {
    return "image/webp"
  }

  return fallback
}
