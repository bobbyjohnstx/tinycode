package session

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bobbyjohnstx/tinycode/internal/id"
)

// ExportPayload is the JSON shape written by `tinycode export`.
type ExportPayload struct {
	Session  Info      `json:"session"`
	Messages []Message `json:"messages"`
}

// Redact returns `[redacted:type:id]` for non-empty values.
func Redact(kind, id, value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	return fmt.Sprintf("[redacted:%s:%s]", kind, id)
}

var absPathRe = regexp.MustCompile(`(?i)(/[^ \t\n"']+|([A-Za-z]:\\[^ \t\n"']+))`)

// SanitizeExport redacts sensitive content in a deep copy of the payload.
func SanitizeExport(in ExportPayload) ExportPayload {
	out := in
	out.Session = in.Session
	out.Session.Title = Redact("session-title", in.Session.ID, in.Session.Title)
	out.Session.Directory = Redact("session-directory", in.Session.ID, in.Session.Directory)
	out.Messages = make([]Message, len(in.Messages))
	for i, msg := range in.Messages {
		out.Messages[i] = sanitizeMessage(msg)
	}
	return out
}

func sanitizeMessage(msg Message) Message {
	out := msg
	out.Parts = make([]Part, len(msg.Parts))
	for i, p := range msg.Parts {
		out.Parts[i] = sanitizePart(p, msg.ID)
	}
	return out
}

func sanitizePart(p Part, msgID string) Part {
	id := p.ToolCallID
	if id == "" {
		id = msgID
	}
	switch p.Type {
	case PartText:
		p.Text = Redact("text", id, p.Text)
	case PartReasoning:
		p.Text = Redact("reasoning", id, p.Text)
	case PartToolCall:
		p.ToolArgs = Redact("tool-args", id, p.ToolArgs)
	case PartToolResult:
		p.ToolResult = Redact("tool-output", id, p.ToolResult)
	case PartFile:
		p.FilePath = Redact("file-path", id, p.FilePath)
		p.FileData = Redact("file-data", id, p.FileData)
		p.Text = Redact("file-text", id, p.Text)
	case PartImage:
		p.ImageData = Redact("image-data", id, p.ImageData)
	}
	// Also redact leftover absolute paths in text fields.
	p.Text = redactAbsPaths(p.Text, id)
	p.ToolArgs = redactAbsPaths(p.ToolArgs, id)
	p.ToolResult = redactAbsPaths(p.ToolResult, id)
	return p
}

func redactAbsPaths(s, id string) string {
	if s == "" || (!strings.Contains(s, "/") && !strings.Contains(s, `\`)) {
		return s
	}
	return absPathRe.ReplaceAllStringFunc(s, func(match string) string {
		base := filepath.Base(match)
		return Redact("path", id+"-"+base, match)
	})
}

// RemapImportMessages mints new message IDs bound to newSessionID.
// Always creates fresh IDs — never reuses export IDs.
func RemapImportMessages(newSessionID string, msgs []Message) ([]Message, error) {
	out := make([]Message, len(msgs))
	for i, msg := range msgs {
		newMsgID, err := id.Ascending("message")
		if err != nil {
			return nil, fmt.Errorf("generating message ID: %w", err)
		}
		msg.ID = newMsgID
		msg.SessionID = newSessionID
		if msg.CreatedAt.IsZero() {
			msg.CreatedAt = time.Now().UTC()
		}
		out[i] = msg
	}
	return out, nil
}
