package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var oscHexFragment = regexp.MustCompile(`^[0-9a-fA-F]{1,4}(/[0-9a-fA-F]{1,4}){1,2}\\?$`)

var (
	colorPromptSurface lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "#F0F0F0", Dark: "#1E293B"}
	colorPromptDim     lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "#999999", Dark: "#777777"}
	colorPromptPrimary lipgloss.TerminalColor = lipgloss.AdaptiveColor{Light: "#0070F3", Dark: "#58A6FF"}
)

// PromptInput is a multi-line text input with a metadata bar,
// autocomplete popover, and prompt history navigation.
type PromptInput struct {
	textarea      textarea.Model
	autocomplete  Autocomplete
	fileComplete  FileCompleter
	history       PromptHistory
	agent         string
	model         string
	provider      string
	agentColor    lipgloss.AdaptiveColor
	thinkingLevel string
	effortLevel   string
	cycleAgents   []string
	width         int
	cwd           string
	keys          KeyMap
	guardEnabled  bool
	startTime     time.Time
	imageCount    int
	imageSize     int // total bytes of attached images
}

// NewPromptInput creates a PromptInput with the given width.
func NewPromptInput(width int) PromptInput {
	ta := textarea.New()
	ta.Placeholder = `Ask anything... "Fix a TODO in the codebase"`
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.SetWidth(width - 6)
	ta.SetHeight(3)
	ta.Focus()
	ta.CharLimit = 0

	noBorder := lipgloss.NewStyle()
	ta.FocusedStyle.Base = noBorder
	ta.FocusedStyle.CursorLine = noBorder
	ta.FocusedStyle.CursorLineNumber = noBorder
	ta.FocusedStyle.EndOfBuffer = noBorder
	ta.FocusedStyle.Prompt = noBorder
	ta.BlurredStyle.Base = noBorder
	ta.BlurredStyle.CursorLine = noBorder
	ta.BlurredStyle.CursorLineNumber = noBorder
	ta.BlurredStyle.EndOfBuffer = noBorder
	ta.BlurredStyle.Prompt = noBorder

	return PromptInput{
		textarea:     ta,
		autocomplete: NewAutocomplete(),
		history:      NewPromptHistory(),
		agent:        "build",
		agentColor:   AgentColor("build"),
		width:        width,
		keys:         DefaultKeyMap(),
	}
}

// EnableStartupGuard arms the startup guard. The actual timer starts
// when ResetStartupGuard is called (typically on first render).
func (p *PromptInput) EnableStartupGuard() {
	p.guardEnabled = true
	p.startTime = time.Now()
}

// ResetStartupGuard restarts the guard timer if the guard was armed.
func (p *PromptInput) ResetStartupGuard() {
	if p.guardEnabled {
		p.startTime = time.Now()
	}
}

// SetSize updates the prompt width.
func (p *PromptInput) SetSize(width int) {
	p.width = width
	p.textarea.SetWidth(width - 6)
	p.autocomplete.SetWidth(width)
	p.fileComplete.SetWidth(width)
}

// SetCwd updates the working directory for file completion.
func (p *PromptInput) SetCwd(cwd string) {
	p.cwd = cwd
	p.fileComplete.SetCwd(cwd)
}

// SetCommands sets the available slash commands for autocomplete.
func (p *PromptInput) SetCommands(cmds []AutocompleteItem) {
	p.autocomplete.SetCommands(cmds)
	p.autocomplete.SetWidth(p.width)
}

// SetAgents sets the available agents for /ask autocomplete.
func (p *PromptInput) SetAgents(agents []AutocompleteItem) {
	p.autocomplete.SetAgents(agents)
}

// SetCycleAgents sets the ordered list of agent names for tab cycling.
func (p *PromptInput) SetCycleAgents(names []string) {
	p.cycleAgents = names
}

// SetMetadata updates the agent/model display below the textarea.
func (p *PromptInput) SetMetadata(agent, model, provider string) {
	p.agent = agent
	p.model = model
	p.provider = provider
	p.agentColor = AgentColor(agent)
}

// SetThinkingLevel updates the thinking level display.
func (p *PromptInput) SetThinkingLevel(level string) {
	p.thinkingLevel = level
}

// SetEffortLevel updates the effort level display.
func (p *PromptInput) SetEffortLevel(level string) {
	p.effortLevel = level
}

// AddImage records that an image has been attached to the prompt.
func (p *PromptInput) AddImage(sizeBytes int) {
	p.imageCount++
	p.imageSize += sizeBytes
}

// ClearImages removes all attached image indicators.
func (p *PromptInput) ClearImages() {
	p.imageCount = 0
	p.imageSize = 0
}

// HasImages returns whether any images are attached.
func (p *PromptInput) HasImages() bool {
	return p.imageCount > 0
}

// Value returns the current text content.
func (p *PromptInput) Value() string {
	return p.textarea.Value()
}

// SetValue replaces the textarea content with the given string.
func (p *PromptInput) SetValue(s string) {
	p.textarea.SetValue(s)
}

// Reset clears the textarea content.
func (p *PromptInput) Reset() {
	p.textarea.Reset()
}

// Focus gives keyboard focus to the textarea.
func (p *PromptInput) Focus() {
	p.textarea.Focus()
}

// Blur removes keyboard focus from the textarea.
func (p *PromptInput) Blur() {
	p.textarea.Blur()
}

// Init implements tea.Model.
func (p PromptInput) Init() tea.Cmd {
	return textarea.Blink
}

// Update implements tea.Model.
func (p PromptInput) Update(msg tea.Msg) (PromptInput, tea.Cmd) {
	// Handle FileCompletionMsg for async file listing results.
	if fcMsg, ok := msg.(FileCompletionMsg); ok {
		p.fileComplete.SetResults(fcMsg.Items, fcMsg.Query)
		return p, nil
	}

	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return p.forwardToTextarea(msg)
	}

	// Discard terminal escape sequences that leak through bubbletea's input parser.
	if s := keyMsg.String(); isTerminalEscape(s) {
		return p, nil
	}
	// Discard rune-only input during startup grace period — terminal
	// responses (OSC color, CSI cursor reports) arrive as printable
	// characters indistinguishable from typing.
	if keyMsg.Type == tea.KeyRunes && p.guardEnabled && time.Since(p.startTime) < 2*time.Second {
		return p, nil
	}
	if p.fileComplete.IsVisible() {
		if result, cmd, handled := p.handleFileCompleteKey(keyMsg); handled {
			return result, cmd
		}
	}
	if p.autocomplete.IsVisible() {
		if result, cmd, handled := p.handleAutocompleteKey(keyMsg); handled {
			return result, cmd
		}
	}

	switch keyMsg.String() {
	case "enter":
		// Submit on Enter (plain, no modifier).
		content := p.textarea.Value()
		if content != "" {
			p.history.Add(content)
			p.textarea.Reset()
			return p, func() tea.Msg {
				return PromptSubmittedMsg{Content: content}
			}
		}
		return p, nil

	case "shift+enter", "alt+enter":
		// Insert newline.
		p.textarea, _ = p.textarea.Update(msg)
		return p, nil

	case "tab":
		if len(p.cycleAgents) > 0 {
			next := p.nextAgent(1)
			return p, func() tea.Msg {
				return AgentSelectedMsg{Agent: next}
			}
		}
		return p, nil

	case "shift+tab":
		if len(p.cycleAgents) > 0 {
			next := p.nextAgent(-1)
			return p, func() tea.Msg {
				return AgentSelectedMsg{Agent: next}
			}
		}
		return p, nil

	case "up":
		// Navigate history when autocomplete is not visible.
		if prev, ok := p.history.Previous(p.textarea.Value()); ok {
			p.textarea.SetValue(prev)
			return p, nil
		}
		return p, nil

	case "down":
		// Navigate history forward.
		if next, ok := p.history.Next(p.textarea.Value()); ok {
			p.textarea.SetValue(next)
		} else {
			p.textarea.SetValue(p.history.StashValue())
		}
		return p, nil
	}

	return p.forwardToTextarea(msg)
}

// forwardToTextarea passes a message to the underlying textarea and updates autocomplete/file completion.
func (p PromptInput) forwardToTextarea(msg tea.Msg) (PromptInput, tea.Cmd) {
	var cmd tea.Cmd
	p.textarea, cmd = p.textarea.Update(msg)
	text := p.textarea.Value()

	// Compute cursor offset from textarea position.
	cursorOffset := p.cursorOffset()

	// Try file completion first.
	fcCmd := p.fileComplete.UpdateAtCursor(text, cursorOffset)
	if p.fileComplete.IsVisible() {
		// File completer is active — skip slash autocomplete.
		if fcCmd != nil {
			return p, tea.Batch(cmd, fcCmd)
		}
		return p, cmd
	}

	// No file completion — update slash autocomplete as before.
	p.autocomplete.UpdateInput(text)
	if fcCmd != nil {
		return p, tea.Batch(cmd, fcCmd)
	}
	return p, cmd
}

// cursorOffset computes the absolute rune offset of the textarea cursor.
func (p PromptInput) cursorOffset() int {
	value := p.textarea.Value()
	row := p.textarea.Line()
	li := p.textarea.LineInfo()
	col := li.CharOffset

	lines := strings.Split(value, "\n")
	offset := 0
	for i := 0; i < row && i < len(lines); i++ {
		offset += len([]rune(lines[i])) + 1 // +1 for newline
	}
	if row < len(lines) {
		lineRunes := []rune(lines[row])
		if col > len(lineRunes) {
			col = len(lineRunes)
		}
		offset += col
	}
	return offset
}

// View implements tea.Model.
func (p PromptInput) View() string {
	accentColor := p.agentColor
	surfaceColor := colorPromptSurface

	innerWidth := p.width - 3 // ┃ + padding

	taView := p.textarea.View()
	meta := p.renderMetadata()

	composerBody := lipgloss.JoinVertical(lipgloss.Left, taView, meta)
	paddedBody := lipgloss.NewStyle().PaddingLeft(1).Width(innerWidth).Render(composerBody)

	// Separator line above prompt
	spacerLine := lipgloss.NewStyle().
		Foreground(colorPromptPrimary).
		Render(strings.Repeat("─", p.width))

	// Left border: ┃ with accent color, indented from left edge
	indent := "  "
	bodyLines := strings.Split(paddedBody, "\n")
	borderChar := lipgloss.NewStyle().Foreground(accentColor).Render("┃")
	var bordered []string
	bordered = append(bordered, spacerLine)
	for _, line := range bodyLines {
		bordered = append(bordered, indent+borderChar+line)
	}

	// Bottom: indented ╹ + ▀▀▀▀ fill
	bottomLeft := lipgloss.NewStyle().Foreground(accentColor).Render("╹")
	fillWidth := p.width - 3
	if fillWidth < 1 {
		fillWidth = 1
	}
	fillChar := lipgloss.NewStyle().Foreground(surfaceColor).Render(strings.Repeat("▀", fillWidth))
	bordered = append(bordered, indent+bottomLeft+fillChar)

	result := strings.Join(bordered, "\n")

	if p.fileComplete.IsVisible() {
		fcView := p.fileComplete.View()
		return lipgloss.JoinVertical(lipgloss.Left, fcView, result)
	}

	if p.autocomplete.IsVisible() {
		acView := p.autocomplete.View()
		return lipgloss.JoinVertical(lipgloss.Left, acView, result)
	}

	return result
}

// renderMetadata renders the status line below the textarea.
func (p PromptInput) renderMetadata() string {
	dimColor := colorPromptDim

	agentStyle := lipgloss.NewStyle().Foreground(p.agentColor)
	dimStyle := lipgloss.NewStyle().Foreground(dimColor)

	agentName := p.agent
	if len(agentName) > 0 {
		runes := []rune(agentName)
		runes[0] = unicode.ToUpper(runes[0])
		agentName = string(runes)
	}
	agent := agentStyle.Render(agentName)

	modelInfo := truncatedModelProvider(p.model, p.provider, p.width-len(p.agent)-10)
	if modelInfo == "" {
		modelInfo = "No provider selected · /connect"
	}

	result := fmt.Sprintf("  %s %s %s", agent, dimStyle.Render("·"), dimStyle.Render(modelInfo))
	if p.thinkingLevel != "" && p.thinkingLevel != "off" {
		result += "  " + dimStyle.Render("thinking:"+p.thinkingLevel)
	}
	if p.effortLevel != "" && p.effortLevel != "medium" {
		result += "  " + dimStyle.Render("effort:"+p.effortLevel)
	}
	if p.imageCount > 0 {
		label := formatImageSize(p.imageSize)
		if p.imageCount == 1 {
			result += "  " + dimStyle.Render("[image: "+label+"]")
		} else {
			result += "  " + dimStyle.Render(fmt.Sprintf("[%d images: %s]", p.imageCount, label))
		}
	}
	return result
}

// handleAutocompleteKey handles key events when the autocomplete popover is
// visible. Returns (model, cmd, true) if the key was consumed.
func (p PromptInput) handleAutocompleteKey(keyMsg tea.KeyMsg) (PromptInput, tea.Cmd, bool) {
	switch keyMsg.String() {
	case "up", "down", "esc":
		var acCmd tea.Cmd
		p.autocomplete, acCmd, _ = p.autocomplete.Update(keyMsg)
		return p, acCmd, true
	case "tab":
		selected := p.autocomplete.Selected()
		if selected != "" {
			p.textarea.SetValue(p.formatSelection(selected))
		}
		p.autocomplete, _, _ = p.autocomplete.Update(keyMsg)
		return p, nil, true
	case "enter":
		selected := p.autocomplete.Selected()
		if selected != "" {
			current := strings.TrimSpace(p.textarea.Value())
			formatted := strings.TrimSpace(p.formatSelection(selected))
			if current != formatted && !strings.HasPrefix(current, formatted+" ") {
				p.textarea.SetValue(p.formatSelection(selected))
				p.autocomplete, _, _ = p.autocomplete.Update(keyMsg)
				return p, nil, true
			}
			p.autocomplete, _, _ = p.autocomplete.Update(keyMsg)
		}
	}
	return p, nil, false
}

// handleFileCompleteKey handles key events when the file completer is visible.
// Returns (model, cmd, true) if the key was consumed.
func (p PromptInput) handleFileCompleteKey(keyMsg tea.KeyMsg) (PromptInput, tea.Cmd, bool) {
	switch keyMsg.String() {
	case "up", "down", "esc":
		var fcCmd tea.Cmd
		p.fileComplete, fcCmd, _ = p.fileComplete.Update(keyMsg)
		return p, fcCmd, true
	case "tab", "enter":
		selected, ok := p.fileComplete.Selected()
		if !ok {
			p.fileComplete, _, _ = p.fileComplete.Update(keyMsg)
			return p, nil, true
		}
		start, end := p.fileComplete.TokenBounds()
		text := p.textarea.Value()
		runes := []rune(text)

		if selected.IsDir {
			// Replace @token with @dir/ and trigger new listing
			newToken := "@" + selected.Path + "/"
			newRunes := make([]rune, 0, len(runes)-end+start+len([]rune(newToken)))
			newRunes = append(newRunes, runes[:start]...)
			newRunes = append(newRunes, []rune(newToken)...)
			newRunes = append(newRunes, runes[end:]...)
			newText := string(newRunes)
			p.textarea.SetValue(newText)

			// Update file completer for the new directory
			cursorOffset := start + len([]rune(newToken))
			fcCmd := p.fileComplete.UpdateAtCursor(newText, cursorOffset)
			return p, fcCmd, true
		}

		// File selected: replace @token with @filepath + trailing space
		newToken := "@" + selected.Path + " "
		newRunes := make([]rune, 0, len(runes)-end+start+len([]rune(newToken)))
		newRunes = append(newRunes, runes[:start]...)
		newRunes = append(newRunes, []rune(newToken)...)
		newRunes = append(newRunes, runes[end:]...)
		p.textarea.SetValue(string(newRunes))

		// Dismiss the file completer
		p.fileComplete.visible = false
		p.fileComplete.cursor = 0
		p.fileComplete.query = ""
		p.fileComplete.lastQuery = nil
		return p, nil, true
	}
	return p, nil, false
}

// formatSelection returns the text to fill into the prompt for a selected
// autocomplete item, accounting for the current mode ("/" vs "/ask").
func (p PromptInput) formatSelection(selected string) string {
	if p.autocomplete.Mode() == "/ask" {
		return "/ask " + selected + " "
	}
	return "/" + selected + " "
}

// nextAgent returns the agent name dir positions from the current agent.
func (p *PromptInput) nextAgent(dir int) string {
	if len(p.cycleAgents) == 0 {
		return p.agent
	}
	idx := 0
	for i, name := range p.cycleAgents {
		if name == p.agent {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(p.cycleAgents)) % len(p.cycleAgents)
	return p.cycleAgents[idx]
}

// formatImageSize returns a human-readable size string.
func formatImageSize(bytes int) string {
	if bytes < 1024 {
		return fmt.Sprintf("%dB PNG", bytes)
	}
	kb := bytes / 1024
	if kb < 1024 {
		return fmt.Sprintf("%dKB PNG", kb)
	}
	mb := float64(bytes) / (1024 * 1024)
	return fmt.Sprintf("%.1fMB PNG", mb)
}

// isTerminalEscape returns true if the string looks like a terminal escape
// response that leaked through bubbletea's input parser (raw ESC/C1 bytes
// or OSC color response fragments like "rgb:" or "11;").
func isTerminalEscape(s string) bool {
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
	return false
}
