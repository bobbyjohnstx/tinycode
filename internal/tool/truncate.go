package tool

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	PreviewHeadLines = 30
	PreviewTailLines = 20

	// MaxOutputSize caps the bytes buffered from shell commands and file
	// reads to prevent OOM when a process produces unbounded output.
	MaxOutputSize = 10 * 1024 * 1024 // 10MB
)

// Truncation limits. Defaults match historical constants; ConfigureOutputLimits
// may override them from config.tool_output.
var (
	MaxLines        = 2000
	MaxBytes        = 50 * 1024 // 50KB
	PreviewMaxBytes = 50 * 1024 // 50KB
)

// ConfigureOutputLimits applies tool_output max_lines / max_bytes from config.
// Zero or negative values are ignored.
func ConfigureOutputLimits(maxLines, maxBytes int) {
	if maxLines > 0 {
		MaxLines = maxLines
	}
	if maxBytes > 0 {
		MaxBytes = maxBytes
		PreviewMaxBytes = maxBytes
	}
}

// LimitedWriter wraps a bytes.Buffer and silently discards writes after max
// bytes. It always returns len(p), nil so exec.Cmd does not abort on write
// errors.
type LimitedWriter struct {
	buf      bytes.Buffer
	max      int
	Overflow bool
}

// NewLimitedWriter returns a LimitedWriter that captures at most max bytes.
func NewLimitedWriter(max int) *LimitedWriter {
	return &LimitedWriter{max: max}
}

func (w *LimitedWriter) Write(p []byte) (int, error) {
	if w.Overflow {
		return len(p), nil
	}
	remaining := w.max - w.buf.Len()
	if remaining <= 0 {
		w.Overflow = true
		return len(p), nil
	}
	if len(p) > remaining {
		w.buf.Write(p[:remaining])
		w.Overflow = true
		return len(p), nil
	}
	w.buf.Write(p)
	return len(p), nil
}

// Bytes returns the captured bytes.
func (w *LimitedWriter) Bytes() []byte { return w.buf.Bytes() }

// Len returns the number of captured bytes.
func (w *LimitedWriter) Len() int { return w.buf.Len() }

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
