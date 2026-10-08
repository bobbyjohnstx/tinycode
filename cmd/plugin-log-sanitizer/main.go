package main

import (
	"context"
	"regexp"

	"github.com/bobbyjohnstx/tinycode/pkg/plugin"
)

// sensitivePatterns defines regex patterns for secrets and sensitive data.
// Each pattern replaces matched content with [REDACTED].
var sensitivePatterns = []*regexp.Regexp{
	// API keys
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`(?i)\bapi[_-]?key\s*[=:]\s*"(?:[^"\\]|\\.)*"`),
	regexp.MustCompile(`(?i)\bapi[_-]?key\s*[=:]\s*'(?:[^'\\]|\\.)*'`),
	regexp.MustCompile(`(?i)\bapi[_-]?key\s*[=:]\s*\S+`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bghp_[A-Za-z0-9]{36,}\b`),
	regexp.MustCompile(`\bgho_[A-Za-z0-9]{36,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]+\b`),
	regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`\bsha256~[A-Za-z0-9_-]+\b`),

	// Bearer tokens and bare JWTs
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9_\-.]{20,}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`),
	regexp.MustCompile(`(?i)\btoken\s*[=:]\s*"(?:[^"\\]|\\.)*"`),
	regexp.MustCompile(`(?i)\btoken\s*[=:]\s*'(?:[^'\\]|\\.)*'`),
	regexp.MustCompile(`(?i)\btoken\s*[=:]\s*\S+`),

	// Passwords
	regexp.MustCompile(`(?i)\b(?:password|passwd|PASS)\s*[=:]\s*"(?:[^"\\]|\\.)*"`),
	regexp.MustCompile(`(?i)\b(?:password|passwd|PASS)\s*[=:]\s*'(?:[^'\\]|\\.)*'`),
	regexp.MustCompile(`(?i)\b(?:password|passwd|PASS)\s*[=:]\s*\S+`),

	// Private keys
	regexp.MustCompile(`-----BEGIN (?:OPENSSH |ENCRYPTED |RSA |EC |DSA )?PRIVATE KEY-----[\s\S]*?-----END (?:OPENSSH |ENCRYPTED |RSA |EC |DSA )?PRIVATE KEY-----`),

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
			ToolExecBefore: func(_ context.Context, input plugin.ToolExecBeforeInput) (*plugin.ToolExecBeforeOutput, error) {
				sanitized := sanitize(input.ToolArgs)
				if sanitized == input.ToolArgs {
					return nil, nil
				}
				return &plugin.ToolExecBeforeOutput{ToolArgs: sanitized}, nil
			},
			ToolExecAfter: func(_ context.Context, input plugin.ToolExecAfterInput) (*plugin.ToolExecAfterOutput, error) {
				sanitized := sanitize(input.Output)
				if sanitized == input.Output {
					return nil, nil
				}
				return &plugin.ToolExecAfterOutput{
					Output:  sanitized,
					IsError: input.IsError,
				}, nil
			},
		},
	})
}
