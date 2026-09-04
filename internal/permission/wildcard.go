package permission

import (
	"regexp"
	"runtime"
	"strings"
)

func WildcardMatch(input, pattern string) bool {
	normalized := strings.ReplaceAll(input, "\\", "/")
	escaped := strings.ReplaceAll(pattern, "\\", "/")

	escaped = regexp.QuoteMeta(escaped)
	escaped = strings.ReplaceAll(escaped, `\*`, ".*")
	escaped = strings.ReplaceAll(escaped, `\?`, ".")

	if strings.HasSuffix(escaped, " .*") {
		escaped = escaped[:len(escaped)-3] + "( .*)?"
	}

	flags := "(?s)"
	if runtime.GOOS == "windows" {
		flags = "(?si)"
	}

	re, err := regexp.Compile(flags + "^" + escaped + "$")
	if err != nil {
		return false
	}
	return re.MatchString(normalized)
}
