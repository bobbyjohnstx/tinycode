package main

import (
	"context"
	"regexp"

	"github.com/bobbyjohnstx/tinycode-go/pkg/plugin"
)

// sensitivePatterns defines regex patterns for secrets and sensitive data.
// Each pattern replaces matched content with [REDACTED].
var sensitivePatterns = []*regexp.Regexp{
	// API keys
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`(?i)\bapi[_-]?key\s*[=:]\s*\S+`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bghp_[A-Za-z0-9]{36,}\b`),
	regexp.MustCompile(`\bgho_[A-Za-z0-9]{36,}\b`),
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`),

	// Bearer tokens
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9_\-.]{20,}\b`),
	regexp.MustCompile(`(?i)\btoken\s*[=:]\s*\S+`),

	// Passwords
	regexp.MustCompile(`(?i)\b(?:password|passwd|PASS)\s*[=:]\s*\S+`),

	// Private keys
	regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA )?PRIVATE KEY-----[\s\S]*?-----END (?:RSA |EC |DSA )?PRIVATE KEY-----`),

	// Connection strings with embedded credentials
	regexp.MustCompile(`://[^/\s]+:[^@/\s]+@`),

	// AWS secret access keys
	regexp.MustCompile(`(?i)aws_secret_access_key\s*[=:]\s*[A-Za-z0-9/+=]{40}\b`),
}

// sanitize replaces all sensitive patterns in text with [REDACTED].
func sanitize(text string) string {
	result := text
	for _, p := range sensitivePatterns {
		result = p.ReplaceAllString(result, "[REDACTED]")
	}
	return result
}

func main() {
	plugin.Run(plugin.Plugin{
		ID: "log-sanitizer",
		Hooks: plugin.HookHandlers{
			ToolExecAfter: func(_ context.Context, input plugin.ToolExecAfterInput) error {
				// Detect sensitive content in tool output.
				// Note: the current ToolExecAfter hook signature returns only
				// error, so output cannot be modified through the protocol.
				// The sanitize function is validated via unit tests.
				_ = sanitize(input.Output)
				return nil
			},
		},
	})
}
