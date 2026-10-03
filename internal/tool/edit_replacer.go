package tool

import (
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"unicode"
)

// Replacer defines a fuzzy replacement strategy.
type Replacer interface {
	// Replace attempts to replace oldString with newString in content.
	// Returns the new content and true if successful, or ("", false) if not.
	Replace(content, oldString, newString string, replaceAll bool) (string, bool)
	// Name returns a human-readable name for logging.
	Name() string
}

// replacerChain is the ordered cascade of replacement strategies.
var replacerChain = []Replacer{
	&SimpleReplacer{},
	&LineTrimmedReplacer{},
	&BlockAnchorReplacer{Threshold: 0.0},
	&WhitespaceNormalizedReplacer{},
	&IndentationFlexibleReplacer{},
	&EscapeNormalizedReplacer{},
	&TrimmedBoundaryReplacer{},
	&ContextAwareReplacer{},
	&BlockAnchorReplacer{Threshold: 0.3},
	&MultiOccurrenceReplacer{},
}

// ErrMultipleExactMatches is returned when oldString appears multiple times
// and replaceAll is false.
var ErrMultipleExactMatches = fmt.Errorf("multiple exact matches")

// cascadeReplace tries each replacer in order and returns the first success.
// Returns (newContent, strategyName, matchCount, ok).
// When matchCount > 1 and ok is false, the caller should report the count.
func cascadeReplace(content, oldString, newString string, replaceAll bool) (string, string, int, bool) {
	// Check for multiple exact matches upfront so we can report count.
	exactCount := strings.Count(content, oldString)
	if exactCount > 1 && !replaceAll {
		return "", "", exactCount, false
	}

	for _, r := range replacerChain {
		result, ok := r.Replace(content, oldString, newString, replaceAll)
		if ok {
			slog.Info("edit replacer matched", "strategy", r.Name())
			return result, r.Name(), 1, true
		}
	}
	return "", "", 0, false
}

// --- Strategy 1: SimpleReplacer (exact match) ---

type SimpleReplacer struct{}

func (r *SimpleReplacer) Name() string { return "exact" }

func (r *SimpleReplacer) Replace(content, oldString, newString string, replaceAll bool) (string, bool) {
	count := strings.Count(content, oldString)
	if count == 0 {
		return "", false
	}
	if !replaceAll && count > 1 {
		return "", false
	}
	if replaceAll {
		return strings.ReplaceAll(content, oldString, newString), true
	}
	return strings.Replace(content, oldString, newString, 1), true
}

// --- Strategy 2: LineTrimmedReplacer ---

type LineTrimmedReplacer struct{}

func (r *LineTrimmedReplacer) Name() string { return "line-trimmed" }

func (r *LineTrimmedReplacer) Replace(content, oldString, newString string, replaceAll bool) (string, bool) {
	contentLines := strings.Split(content, "\n")
	needleLines := strings.Split(oldString, "\n")
	if len(needleLines) == 0 {
		return "", false
	}

	matches := findTrimmedMatches(contentLines, needleLines)
	if len(matches) == 0 {
		return "", false
	}
	if !replaceAll && len(matches) > 1 {
		return "", false
	}

	// Replace from last to first to preserve indices.
	result := make([]string, len(contentLines))
	copy(result, contentLines)

	for i := len(matches) - 1; i >= 0; i-- {
		idx := matches[i]
		replacementLines := strings.Split(newString, "\n")
		after := append([]string{}, result[idx+len(needleLines):]...)
		result = append(result[:idx], replacementLines...)
		result = append(result, after...)

		if !replaceAll {
			break
		}
	}

	return strings.Join(result, "\n"), true
}

func findTrimmedMatches(contentLines, needleLines []string) []int {
	trimmedNeedle := make([]string, len(needleLines))
	for i, l := range needleLines {
		trimmedNeedle[i] = strings.TrimSpace(l)
	}

	var matches []int
	for i := 0; i <= len(contentLines)-len(needleLines); i++ {
		match := true
		for j, tn := range trimmedNeedle {
			if strings.TrimSpace(contentLines[i+j]) != tn {
				match = false
				break
			}
		}
		if match {
			matches = append(matches, i)
		}
	}
	return matches
}

// --- Strategy 3: BlockAnchorReplacer ---

type BlockAnchorReplacer struct {
	Threshold float64 // minimum similarity for interior lines
}

func (r *BlockAnchorReplacer) Name() string {
	if r.Threshold > 0 {
		return "block-anchor-relaxed"
	}
	return "block-anchor"
}

func (r *BlockAnchorReplacer) Replace(content, oldString, newString string, replaceAll bool) (string, bool) {
	contentLines := strings.Split(content, "\n")
	needleLines := strings.Split(oldString, "\n")
	if len(needleLines) < 2 {
		return "", false
	}

	firstTrimmed := strings.TrimSpace(needleLines[0])
	lastTrimmed := strings.TrimSpace(needleLines[len(needleLines)-1])
	if firstTrimmed == "" || lastTrimmed == "" {
		return "", false
	}

	var matches []int
	for i := 0; i <= len(contentLines)-len(needleLines); i++ {
		if strings.TrimSpace(contentLines[i]) != firstTrimmed {
			continue
		}
		endIdx := i + len(needleLines) - 1
		if strings.TrimSpace(contentLines[endIdx]) != lastTrimmed {
			continue
		}

		// Score interior lines.
		if len(needleLines) > 2 {
			totalSim := 0.0
			count := 0
			for j := 1; j < len(needleLines)-1; j++ {
				sim := Similarity(strings.TrimSpace(contentLines[i+j]), strings.TrimSpace(needleLines[j]))
				totalSim += sim
				count++
			}
			avgSim := totalSim / float64(count)
			if avgSim < (1.0 - r.Threshold) {
				if r.Threshold == 0 {
					continue
				}
				// With threshold > 0, allow more fuzzy matches.
				if avgSim < 0.7 {
					continue
				}
			}
		}

		matches = append(matches, i)
	}

	if len(matches) == 0 {
		return "", false
	}
	if !replaceAll && len(matches) > 1 {
		return "", false
	}

	result := make([]string, len(contentLines))
	copy(result, contentLines)

	for i := len(matches) - 1; i >= 0; i-- {
		idx := matches[i]
		replacementLines := strings.Split(newString, "\n")
		after := append([]string{}, result[idx+len(needleLines):]...)
		result = append(result[:idx], replacementLines...)
		result = append(result, after...)

		if !replaceAll {
			break
		}
	}

	return strings.Join(result, "\n"), true
}

// --- Strategy 4: WhitespaceNormalizedReplacer ---

type WhitespaceNormalizedReplacer struct{}

func (r *WhitespaceNormalizedReplacer) Name() string { return "whitespace-normalized" }

var wsRe = regexp.MustCompile(`\s+`)

func normalizeWhitespace(s string) string {
	return strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
}

func (r *WhitespaceNormalizedReplacer) Replace(content, oldString, newString string, replaceAll bool) (string, bool) {
	normOld := normalizeWhitespace(oldString)
	if normOld == "" {
		return "", false
	}

	contentLines := strings.Split(content, "\n")
	needleLines := strings.Split(oldString, "\n")

	matches := findNormalizedMatches(contentLines, needleLines)
	if len(matches) == 0 {
		return "", false
	}
	if !replaceAll && len(matches) > 1 {
		return "", false
	}

	result := make([]string, len(contentLines))
	copy(result, contentLines)

	for i := len(matches) - 1; i >= 0; i-- {
		idx := matches[i]
		replacementLines := strings.Split(newString, "\n")
		after := append([]string{}, result[idx+len(needleLines):]...)
		result = append(result[:idx], replacementLines...)
		result = append(result, after...)

		if !replaceAll {
			break
		}
	}

	return strings.Join(result, "\n"), true
}

func findNormalizedMatches(contentLines, needleLines []string) []int {
	normNeedle := make([]string, len(needleLines))
	for i, l := range needleLines {
		normNeedle[i] = normalizeWhitespace(l)
	}

	var matches []int
	for i := 0; i <= len(contentLines)-len(needleLines); i++ {
		match := true
		for j, nn := range normNeedle {
			if normalizeWhitespace(contentLines[i+j]) != nn {
				match = false
				break
			}
		}
		if match {
			matches = append(matches, i)
		}
	}
	return matches
}

// --- Strategy 5: IndentationFlexibleReplacer ---

type IndentationFlexibleReplacer struct{}

func (r *IndentationFlexibleReplacer) Name() string { return "indentation-flexible" }

func (r *IndentationFlexibleReplacer) Replace(content, oldString, newString string, replaceAll bool) (string, bool) {
	contentLines := strings.Split(content, "\n")
	needleLines := strings.Split(oldString, "\n")
	if len(needleLines) == 0 {
		return "", false
	}

	// Strip common indent from needleLines.
	commonIndent := findCommonIndent(needleLines)
	stripped := make([]string, len(needleLines))
	for i, l := range needleLines {
		if len(l) >= len(commonIndent) {
			stripped[i] = l[len(commonIndent):]
		} else {
			stripped[i] = strings.TrimLeft(l, " \t")
		}
	}

	var matches []int
	for i := 0; i <= len(contentLines)-len(needleLines); i++ {
		contentIndent := getIndent(contentLines[i])
		match := true
		for j, s := range stripped {
			cl := contentLines[i+j]
			clNoIndent := strings.TrimPrefix(cl, contentIndent)
			// If the content line has more indent than the base, trim the base portion.
			if !strings.HasPrefix(cl, contentIndent) {
				clNoIndent = strings.TrimLeft(cl, " \t")
			}
			if clNoIndent != s {
				match = false
				break
			}
		}
		if match {
			matches = append(matches, i)
		}
	}

	if len(matches) == 0 {
		return "", false
	}
	if !replaceAll && len(matches) > 1 {
		return "", false
	}

	result := make([]string, len(contentLines))
	copy(result, contentLines)

	for i := len(matches) - 1; i >= 0; i-- {
		idx := matches[i]
		// Re-indent newString to match the content's indentation.
		contentIndent := getIndent(result[idx])
		newLines := strings.Split(newString, "\n")
		newCommonIndent := findCommonIndent(newLines)
		reindented := make([]string, len(newLines))
		for j, nl := range newLines {
			trimmed := nl
			if len(trimmed) >= len(newCommonIndent) {
				trimmed = nl[len(newCommonIndent):]
			} else {
				trimmed = strings.TrimLeft(nl, " \t")
			}
			if trimmed == "" && strings.TrimSpace(nl) == "" {
				reindented[j] = ""
			} else {
				reindented[j] = contentIndent + trimmed
			}
		}

		after := append([]string{}, result[idx+len(needleLines):]...)
		result = append(result[:idx], reindented...)
		result = append(result, after...)

		if !replaceAll {
			break
		}
	}

	return strings.Join(result, "\n"), true
}

func findCommonIndent(lines []string) string {
	var common *string
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		indent := getIndent(l)
		if common == nil {
			common = &indent
		} else if len(indent) < len(*common) {
			common = &indent
		}
	}
	if common == nil {
		return ""
	}
	return *common
}

func getIndent(line string) string {
	for i, r := range line {
		if r != ' ' && r != '\t' {
			return line[:i]
		}
	}
	return line
}

// --- Strategy 6: EscapeNormalizedReplacer ---

type EscapeNormalizedReplacer struct{}

func (r *EscapeNormalizedReplacer) Name() string { return "escape-normalized" }

func normalizeEscapes(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\t", "    ")
	// Normalize non-breaking spaces.
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == ' ' || r == ' ' || r == '\uFEFF' {
			return ' '
		}
		return r
	}, s)
	return s
}

func (r *EscapeNormalizedReplacer) Replace(content, oldString, newString string, replaceAll bool) (string, bool) {
	normContent := normalizeEscapes(content)
	normOld := normalizeEscapes(oldString)

	count := strings.Count(normContent, normOld)
	if count == 0 {
		return "", false
	}
	if !replaceAll && count > 1 {
		return "", false
	}

	// Apply replacement on normalized content, then return it.
	var result string
	if replaceAll {
		result = strings.ReplaceAll(normContent, normOld, newString)
	} else {
		result = strings.Replace(normContent, normOld, newString, 1)
	}

	return result, true
}

// --- Strategy 7: TrimmedBoundaryReplacer ---

type TrimmedBoundaryReplacer struct{}

func (r *TrimmedBoundaryReplacer) Name() string { return "trimmed-boundary" }

func (r *TrimmedBoundaryReplacer) Replace(content, oldString, newString string, replaceAll bool) (string, bool) {
	// Trim blank first/last lines from oldString and try exact match.
	lines := strings.Split(oldString, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	trimmedOld := strings.Join(lines, "\n")
	if trimmedOld == "" || trimmedOld == oldString {
		return "", false
	}

	count := strings.Count(content, trimmedOld)
	if count == 0 {
		return "", false
	}
	if !replaceAll && count > 1 {
		return "", false
	}

	if replaceAll {
		return strings.ReplaceAll(content, trimmedOld, newString), true
	}
	return strings.Replace(content, trimmedOld, newString, 1), true
}

// --- Strategy 8: ContextAwareReplacer ---

type ContextAwareReplacer struct{}

func (r *ContextAwareReplacer) Name() string { return "context-aware" }

func (r *ContextAwareReplacer) Replace(content, oldString, newString string, replaceAll bool) (string, bool) {
	contentLines := strings.Split(content, "\n")
	needleLines := strings.Split(oldString, "\n")
	if len(needleLines) < 3 {
		return "", false
	}

	// Use non-whitespace-only lines as context anchors.
	type anchor struct {
		offset int
		text   string
	}
	var anchors []anchor
	for i, l := range needleLines {
		t := strings.TrimSpace(l)
		if t != "" && !isAllWhitespace(t) {
			anchors = append(anchors, anchor{offset: i, text: t})
		}
	}
	if len(anchors) < 2 {
		return "", false
	}

	var matches []int
	for i := 0; i <= len(contentLines)-len(needleLines); i++ {
		allMatch := true
		for _, a := range anchors {
			if i+a.offset >= len(contentLines) {
				allMatch = false
				break
			}
			if strings.TrimSpace(contentLines[i+a.offset]) != a.text {
				allMatch = false
				break
			}
		}
		if allMatch {
			matches = append(matches, i)
		}
	}

	if len(matches) == 0 {
		return "", false
	}
	if !replaceAll && len(matches) > 1 {
		return "", false
	}

	result := make([]string, len(contentLines))
	copy(result, contentLines)

	for i := len(matches) - 1; i >= 0; i-- {
		idx := matches[i]
		replacementLines := strings.Split(newString, "\n")
		after := append([]string{}, result[idx+len(needleLines):]...)
		result = append(result[:idx], replacementLines...)
		result = append(result, after...)

		if !replaceAll {
			break
		}
	}

	return strings.Join(result, "\n"), true
}

func isAllWhitespace(s string) bool {
	for _, r := range s {
		if !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// --- Strategy 9: MultiOccurrenceReplacer ---

type MultiOccurrenceReplacer struct{}

func (r *MultiOccurrenceReplacer) Name() string { return "multi-occurrence" }

func (r *MultiOccurrenceReplacer) Replace(content, oldString, newString string, replaceAll bool) (string, bool) {
	if replaceAll {
		return "", false
	}

	contentLines := strings.Split(content, "\n")
	needleLines := strings.Split(oldString, "\n")
	if len(needleLines) == 0 {
		return "", false
	}

	// Find all fuzzy matches using line trimming and score them.
	type scoredMatch struct {
		index int
		score float64
	}

	var candidates []scoredMatch
	for i := 0; i <= len(contentLines)-len(needleLines); i++ {
		totalSim := 0.0
		for j := 0; j < len(needleLines); j++ {
			totalSim += Similarity(
				strings.TrimSpace(contentLines[i+j]),
				strings.TrimSpace(needleLines[j]),
			)
		}
		avgSim := totalSim / float64(len(needleLines))
		if avgSim >= 0.8 {
			candidates = append(candidates, scoredMatch{index: i, score: avgSim})
		}
	}

	if len(candidates) == 0 {
		return "", false
	}

	// Pick the best scoring match.
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	result := make([]string, len(contentLines))
	copy(result, contentLines)

	replacementLines := strings.Split(newString, "\n")
	after := append([]string{}, result[best.index+len(needleLines):]...)
	result = append(result[:best.index], replacementLines...)
	result = append(result, after...)

	return strings.Join(result, "\n"), true
}
