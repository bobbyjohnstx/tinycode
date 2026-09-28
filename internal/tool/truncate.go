package tool

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxLines = 2000
	MaxBytes = 50 * 1024 // 50KB

	PreviewHeadLines = 30
	PreviewTailLines = 20
	PreviewMaxBytes  = 50 * 1024 // 50KB
)

type TruncDirection int

const (
	TruncHead TruncDirection = iota
	TruncTail
)

type TruncateResult struct {
	Content   string
	Truncated bool
	FullSize  int
}

func Truncate(content string, dir TruncDirection) TruncateResult {
	if len(content) <= MaxBytes {
		lines := strings.Split(content, "\n")
		if len(lines) <= MaxLines {
			return TruncateResult{Content: content, FullSize: len(content)}
		}
	}

	fullSize := len(content)
	lines := strings.Split(content, "\n")

	if len(content) > MaxBytes {
		content = truncateBytes(content, dir)
		lines = strings.Split(content, "\n")
	}

	if len(lines) > MaxLines {
		lines = truncateLines(lines, dir)
	}

	truncated := strings.Join(lines, "\n")

	var hint string
	switch dir {
	case TruncHead:
		hint = fmt.Sprintf("\n\n... [truncated %d bytes, showing last %d lines of output] ...", fullSize, len(lines))
		truncated = hint + "\n" + truncated
	case TruncTail:
		hint = fmt.Sprintf("\n\n... [truncated %d bytes, showing first %d lines of output] ...", fullSize, len(lines))
		truncated = truncated + hint
	}

	return TruncateResult{
		Content:   truncated,
		Truncated: true,
		FullSize:  fullSize,
	}
}

func truncateBytes(content string, dir TruncDirection) string {
	switch dir {
	case TruncHead:
		s := content[len(content)-MaxBytes:]
		for len(s) > 0 && !utf8.RuneStart(s[0]) {
			s = s[1:]
		}
		return s
	default:
		s := content[:MaxBytes]
		for len(s) > 0 && !utf8.Valid([]byte(s)) {
			s = s[:len(s)-1]
		}
		return s
	}
}

func truncateLines(lines []string, dir TruncDirection) []string {
	switch dir {
	case TruncHead:
		return lines[len(lines)-MaxLines:]
	default:
		return lines[:MaxLines]
	}
}

// TruncPreview produces a head+tail preview for tool output. Content that fits
// within PreviewHeadLines+PreviewTailLines lines and PreviewMaxBytes is returned
// unchanged. Larger content is reduced to the first PreviewHeadLines and last
// PreviewTailLines lines with a structured header showing total lines and bytes.
func TruncPreview(content string) TruncateResult {
	totalBytes := len(content)

	// Byte-truncate before line splitting to bound memory.
	truncContent := content
	if totalBytes > PreviewMaxBytes {
		truncContent = content[:PreviewMaxBytes]
		// Trim to valid UTF-8 boundary.
		for len(truncContent) > 0 && !utf8.Valid([]byte(truncContent)) {
			truncContent = truncContent[:len(truncContent)-1]
		}
	}

	lines := strings.Split(truncContent, "\n")
	totalLines := len(lines)
	previewLimit := PreviewHeadLines + PreviewTailLines

	if totalLines <= previewLimit && totalBytes <= PreviewMaxBytes {
		return TruncateResult{Content: content, FullSize: totalBytes}
	}

	head := lines
	if len(head) > PreviewHeadLines {
		head = head[:PreviewHeadLines]
	}
	var tail []string
	if totalLines > previewLimit {
		tail = lines[totalLines-PreviewTailLines:]
	}

	header := fmt.Sprintf("\n[preview: %d of %d lines, %s total]\n",
		PreviewHeadLines+len(tail), totalLines, formatBytes(totalBytes))

	var result string
	if len(tail) > 0 {
		result = strings.Join(head, "\n") + header + "...\n" + strings.Join(tail, "\n")
	} else {
		result = strings.Join(head, "\n") + header
	}

	return TruncateResult{
		Content:   result,
		Truncated: true,
		FullSize:  totalBytes,
	}
}

// formatBytes returns a human-readable byte size string.
func formatBytes(b int) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%dKB", b/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
}
