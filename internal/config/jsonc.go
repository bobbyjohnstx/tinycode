package config

import (
	"log/slog"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ParseJSONC strips comments (line and block) and trailing commas from JSONC
// text, returning valid JSON.
func ParseJSONC(input string) (string, error) {
	result := stripComments(input)
	result = stripTrailingCommas(result)
	return result, nil
}

func stripComments(s string) string {
	var out strings.Builder
	out.Grow(len(s))

	inString := false
	i := 0
	for i < len(s) {
		if inString {
			if s[i] == '\\' && i+1 < len(s) {
				out.WriteByte(s[i])
				out.WriteByte(s[i+1])
				i += 2
				continue
			}
			if s[i] == '"' {
				inString = false
			}
			out.WriteByte(s[i])
			i++
			continue
		}

		if s[i] == '"' {
			inString = true
			out.WriteByte(s[i])
			i++
			continue
		}

		if s[i] == '/' && i+1 < len(s) && s[i+1] == '/' {
			// Line comment — skip to end of line
			i += 2
			for i < len(s) && s[i] != '\n' {
				i++
			}
			continue
		}

		if s[i] == '/' && i+1 < len(s) && s[i+1] == '*' {
			// Block comment — skip to */
			i += 2
			for i+1 < len(s) {
				if s[i] == '*' && s[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		out.WriteByte(s[i])
		i++
	}

	return out.String()
}

func stripTrailingCommas(s string) string {
	var out strings.Builder
	out.Grow(len(s))

	inString := false
	i := 0
	for i < len(s) {
		if inString {
			if s[i] == '\\' && i+1 < len(s) {
				out.WriteByte(s[i])
				out.WriteByte(s[i+1])
				i += 2
				continue
			}
			if s[i] == '"' {
				inString = false
			}
			out.WriteByte(s[i])
			i++
			continue
		}

		if s[i] == '"' {
			inString = true
			out.WriteByte(s[i])
			i++
			continue
		}

		if s[i] == ',' {
			j := i + 1
			for j < len(s) && (s[j] == ' ' || s[j] == '\t' || s[j] == '\n' || s[j] == '\r') {
				j++
			}
			if j < len(s) && (s[j] == '}' || s[j] == ']') {
				i++
				continue
			}
		}

		out.WriteByte(s[i])
		i++
	}

	return out.String()
}

// SubstituteEnvVars replaces {env:VAR_NAME} patterns with environment variable values.
func SubstituteEnvVars(text string, env map[string]string) string {
	var b strings.Builder
	b.Grow(len(text))
	i := 0

	for i < len(text) {
		if i+5 < len(text) && text[i:i+5] == "{env:" {
			end := strings.IndexByte(text[i:], '}')
			if end > 0 {
				varName := text[i+5 : i+end]
				if val, ok := env[varName]; ok {
					b.WriteString(val)
				} else if val, ok := lookupEnv(varName); ok {
					b.WriteString(val)
				} else {
					slog.Warn("unresolved env var placeholder", "var", varName)
				}
				i += end + 1
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size <= 1 {
			b.WriteByte(text[i])
			i++
		} else {
			b.WriteRune(r)
			i += size
		}
	}

	return b.String()
}

func lookupEnv(name string) (string, bool) {
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return "", false
		}
	}
	val := os.Getenv(name)
	if val == "" {
		return "", false
	}
	return val, true
}
