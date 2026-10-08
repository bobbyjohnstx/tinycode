package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// UnmarshalToolArgs decodes tool arguments into dest.
// Empty, whitespace, and JSON null arguments leave dest unchanged so optional
// parameters stay at their zero values. Malformed JSON is returned as an error.
func UnmarshalToolArgs(args json.RawMessage, dest any) error {
	trimmed := bytes.TrimSpace(args)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(trimmed, dest); err != nil {
		return fmt.Errorf("parsing args: %w", err)
	}
	return nil
}
