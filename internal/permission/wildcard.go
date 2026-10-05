package permission

import (
	"regexp"
	"runtime"
	"strings"
	"sync"
)

var (
	regexMu    sync.Mutex
	regexCache = make(map[string]*regexp.Regexp)
)

const maxRegexCache = 256

func cachedCompile(pattern string) (*regexp.Regexp, error) {
	regexMu.Lock()
	defer regexMu.Unlock()
	if re, ok := regexCache[pattern]; ok {
		return re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	if len(regexCache) >= maxRegexCache {
		clear(regexCache)
	}
	regexCache[pattern] = re
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
