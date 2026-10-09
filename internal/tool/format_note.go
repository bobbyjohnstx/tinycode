package tool

import "context"

// formatNote runs the configured formatter and returns a line to append to
// the tool output. An empty string means the file was left as written.
func formatNote(ctx context.Context, tc *Context, path string) string {
	if tc == nil || tc.FormatFile == nil {
		return ""
	}
	name, changed, err := tc.FormatFile(ctx, path)
	if err != nil {
		return "\nFormatter failed: " + err.Error()
	}
	if !changed || name == "" {
		return ""
	}
	return "\nFormatted with " + name
}
