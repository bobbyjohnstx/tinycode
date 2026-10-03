package permission

import (
	"regexp"
	"runtime"
	"strings"
	"sync"
)

var regexCache sync.Map // map[string]*regexp.Regexp

func cachedCompile(pattern string) (*regexp.Regexp, error) {
	if cached, ok := regexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	regexCache.Store(pattern, re)
	return re, nil
}

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

	re, err := cachedCompile(flags + "^" + escaped + "$")
	if err != nil {
		return false
	}
	return re.MatchString(normalized)
}
