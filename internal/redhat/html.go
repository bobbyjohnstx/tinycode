package redhat

import (
	"regexp"
	"strings"
)

var (
	scriptRe    = regexp.MustCompile(`(?is)<script[^>]*>[\s\S]*?</script>`)
	styleRe     = regexp.MustCompile(`(?is)<style[^>]*>[\s\S]*?</style>`)
	tagRe       = regexp.MustCompile(`<[^>]+>`)
	whitespaceRe = regexp.MustCompile(`\s+`)
)

func StripHTML(html string) string {
	s := scriptRe.ReplaceAllString(html, "")
	s = styleRe.ReplaceAllString(s, "")
	s = tagRe.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = whitespaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
