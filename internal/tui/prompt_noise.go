package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	oscHexFragment = regexp.MustCompile(`^[0-9a-fA-F]{1,4}(/[0-9a-fA-F]{1,4}){1,2}\\?$`)
	// Core CPR payload after optional mangled CSI/OSC prefixes are stripped.
	cprCore        = regexp.MustCompile(`^\d{0,4};\d{1,4}R$`)
	cprCorePartial = regexp.MustCompile(`^\d{0,4};?\d{0,4}$`)
)

// handleHistoryBrowserKey handles key events when the history browser
// popover is open. Returns (model, cmd, true) if the key was consumed.
func (p PromptInput) handleHistoryBrowserKey(keyMsg tea.KeyMsg) (PromptInput, tea.Cmd, bool) {
	entries := p.history.Entries()
	switch keyMsg.String() {
	case "esc", "ctrl+r":
		p.historyBrowserOn = false
		return p, nil, true
	case "up":
		if p.historyBrowserCursor > 0 {
			p.historyBrowserCursor--
		}
		return p, nil, true
	case "down":
		if p.historyBrowserCursor < len(entries)-1 {
			p.historyBrowserCursor++
		}
		return p, nil, true
	case "enter", "tab":
		if p.historyBrowserCursor >= 0 && p.historyBrowserCursor < len(entries) {
			p.textarea.SetValue(entries[p.historyBrowserCursor])
		}
		p.historyBrowserOn = false
		return p, nil, true
	}
	return p, nil, false
}

// historyBrowserView renders the ctrl+r history browser popover: a scrollable
// list of past prompts, most recent first, with the selected entry highlighted.
func (p PromptInput) historyBrowserView() string {
	entries := p.history.Entries()
	if len(entries) == 0 {
		return ""
	}
	dimStyle := lipgloss.NewStyle().Foreground(colorPromptDim)
	selectedStyle := lipgloss.NewStyle().Foreground(colorPromptPrimary).Bold(true)

	const maxVisible = 8
	start := 0
	if len(entries) > maxVisible {
		start = len(entries) - maxVisible
	}

	maxLineWidth := p.width - 6
	if maxLineWidth < 10 {
		maxLineWidth = 10
	}

	var b strings.Builder
	b.WriteString(dimStyle.Render(fmt.Sprintf("History (%d) — \u2191\u2193 select \u00b7 enter use \u00b7 esc close", len(entries))))
	for i := len(entries) - 1; i >= start; i-- {
		line := strings.ReplaceAll(entries[i], "\n", " \u21b5 ")
		if len([]rune(line)) > maxLineWidth {
			line = string([]rune(line)[:maxLineWidth-1]) + "\u2026"
		}
		prefix := "  "
		style := dimStyle
		if i == p.historyBrowserCursor {
			prefix = "\u25b8 "
			style = selectedStyle
		}
		b.WriteString("\n" + style.Render(prefix+line))
	}
	return b.String()
}

// stripCPRPrefix removes mangled CSI/OSC junk bubbletea leaves when ESC is lost.
// e.g. "][53;1R" / "]\[53;1R" / "[53;1R" → "53;1R"
func stripCPRPrefix(s string) string {
	for {
		switch {
		case strings.HasPrefix(s, "]"):
			s = s[1:]
		case strings.HasPrefix(s, "\\["):
			s = s[2:]
		case strings.HasPrefix(s, "["):
			s = s[1:]
		case strings.HasPrefix(s, "\\"):
			s = s[1:]
		default:
			return s
		}
	}
}

// isTerminalEscape returns true if the string looks like a terminal escape
// response that leaked through bubbletea's input parser (raw ESC/C1 bytes,
// OSC color fragments, or CSI cursor-position reports like ";1R" / "][53;1R").
func isTerminalEscape(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1b || c == 0x9c {
			return true
		}
	}
	if strings.Contains(s, "rgb:") || strings.HasPrefix(s, "]10;") || strings.HasPrefix(s, "]11;") {
		return true
	}
	if oscHexFragment.MatchString(s) {
		return true
	}
	if cprCore.MatchString(stripCPRPrefix(s)) {
		return true
	}
	return false
}

// isTerminalNoise is a broader filter for split CPR/CSI crumbs.
func isTerminalNoise(s string) bool {
	if isTerminalEscape(s) {
		return true
	}
	core := stripCPRPrefix(s)
	if cprCorePartial.MatchString(core) && strings.ContainsAny(s, "][\\;R") {
		return true
	}
	if matched, _ := regexp.MatchString(`^[\d;]{1,8}R?$`, s); matched && strings.ContainsAny(s, ";R") {
		return true
	}
	// Lone CSI/OSC punctuation that starts a report.
	if s == "]" || s == "[" || s == "\\[" || s == "\\" {
		return true
	}
	return false
}

// absorbTerminalNoise drops complete or in-progress CPR/CSI/OSC leaks.
// Returns true when the key should not reach the textarea.
func (p *PromptInput) absorbTerminalNoise(s string) bool {
	if s == "" {
		return false
	}
	noisyWindow := time.Now().Before(p.noiseUntil)
	if isTerminalEscape(s) || (noisyWindow && isTerminalNoise(s)) {
		p.cprBuf = ""
		return true
	}

	cand := p.cprBuf + s
	if cprCore.MatchString(stripCPRPrefix(cand)) {
		p.cprBuf = ""
		return true
	}

	// Buffer CSI-ish prefixes and CPR digits so split deliveries ("[", "53", ";1R")
	// never reach the textarea. Do not buffer bare digits (normal typing).
	core := stripCPRPrefix(cand)
	startsCSI := strings.ContainsAny(cand, "][\\") || strings.HasPrefix(cand, "\\")
	if len(cand) <= 16 && startsCSI && cprCorePartial.MatchString(core) {
		p.cprBuf = cand
		return true
	}
	// During the post-ClearScreen window, also buffer digit/; crumbs that look
	// like a CPR body (ESC[ already consumed by the parser).
	if noisyWindow && len(cand) <= 12 && cprCorePartial.MatchString(cand) && strings.ContainsAny(cand, ";0123456789") {
		p.cprBuf = cand
		return true
	}
	if p.cprBuf != "" {
		p.cprBuf = ""
		if isTerminalNoise(s) || (noisyWindow && isTerminalNoise(cand)) {
			return true
		}
	}
	return false
}

// cprLeakRe matches mangled cursor-position reports that slipped into the buffer.
// Requires a CSI/OSC-ish prefix or a semicolon so plain text like "1R" is kept.
var cprLeakRe = regexp.MustCompile(`(?:[\]\\[]+\d{0,4};\d{1,4}R|\d{0,4};\d{1,4}R)`)

// scrubCPRValue removes leaked CPR fragments from the composer value.
func scrubCPRValue(s string) (string, bool) {
	if s == "" {
		return s, false
	}
	if isTerminalEscape(s) {
		return "", true
	}
	cleaned := cprLeakRe.ReplaceAllString(s, "")
	if cleaned == s {
		return s, false
	}
	return cleaned, true
}
