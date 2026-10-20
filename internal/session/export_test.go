package session

import (
	"strings"
	"testing"
	"time"
)

func TestSanitizeExport_RedactsSensitiveFields(t *testing.T) {
	in := ExportPayload{
		Session: Info{
			ID:        "ses_abc",
			Title:     "Work on /Users/me/secret",
			Directory: "/Users/me/secret",
		},
		Messages: []Message{
			{
				ID:        "msg_1",
				SessionID: "ses_abc",
				Role:      RoleUser,
				CreatedAt: time.Now(),
				Parts: []Part{
					{Type: PartText, Text: "read /Users/me/secret/creds.json"},
					{Type: PartFile, FilePath: "/Users/me/secret/creds.json", FileData: "API_KEY=abc"},
					{Type: PartToolResult, ToolCallID: "tc1", ToolResult: "contents of /tmp/x"},
				},
			},
		},
	}

	out := SanitizeExport(in)
	if !strings.HasPrefix(out.Session.Directory, "[redacted:session-directory:") {
		t.Fatalf("directory = %q", out.Session.Directory)
	}
	if strings.Contains(out.Session.Directory, "/Users/") {
		t.Fatalf("raw path leaked in directory: %q", out.Session.Directory)
	}
	text := out.Messages[0].Parts[0].Text
	if !strings.Contains(text, "[redacted:") {
		t.Fatalf("expected redaction markers in text, got %q", text)
	}
	if strings.Contains(text, "/Users/me/secret") {
		t.Fatalf("raw path leaked in text: %q", text)
	}
	file := out.Messages[0].Parts[1]
	if !strings.HasPrefix(file.FilePath, "[redacted:file-path:") {
		t.Fatalf("filePath = %q", file.FilePath)
	}
	if file.FileData == "API_KEY=abc" {
		t.Fatal("file data not redacted")
	}
	// Original unchanged.
	if in.Session.Directory != "/Users/me/secret" {
		t.Fatal("SanitizeExport mutated input")
	}
}

func TestRemapImportMessages_AlwaysNew(t *testing.T) {
	in := []Message{
		{ID: "msg_old", SessionID: "ses_old", Role: RoleUser, Parts: []Part{TextPart("hi")}},
	}
	out, err := RemapImportMessages("ses_new", in)
	if err != nil {
		t.Fatal(err)
	}
	if out[0].ID == "msg_old" || !strings.HasPrefix(out[0].ID, "msg_") {
		t.Fatalf("message ID = %q", out[0].ID)
	}
	if out[0].SessionID != "ses_new" {
		t.Fatalf("message SessionID = %q", out[0].SessionID)
	}
}
