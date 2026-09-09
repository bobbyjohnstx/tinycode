package tool

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxLines = 2000
	MaxBytes = 50 * 1024 // 50KB
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
