package procenv

import (
	"os"
	"sort"
	"strings"
)

// Child returns the current process environment with credential-like
// variables removed. extra is applied afterward, so a configured env map
// can pass a value to one child on purpose. The result is never nil:
// a nil Env on exec.Cmd inherits the parent environment.
func Child(extra map[string]string) []string {
	out := make([]string, 0, len(os.Environ()))
	index := map[string]int{}
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || SecretName(name) {
			continue
		}
		index[name] = len(out)
		out = append(out, entry)
	}

	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := name + "=" + extra[name]
		if i, ok := index[name]; ok {
			out[i] = entry
			continue
		}
		index[name] = len(out)
		out = append(out, entry)
	}
	return out
}

// SecretName reports whether an environment variable name looks like a
// credential. Matching is case-insensitive.
func SecretName(name string) bool {
	n := strings.ToUpper(name)
	switch n {
	case "AUTHORIZATION", "AUTH_HEADER", "SECRET", "TOKEN", "PASSWORD":
		return true
	}
	suffixes := []string{
		"_API_KEY",
		"_APIKEY",
		"_SECRET",
		"_TOKEN",
		"_PASSWORD",
		"_PASSWD",
		"_CREDENTIAL",
		"_CREDENTIALS",
		"_PRIVATE_KEY",
		"_ACCESS_KEY",
	}
	for _, suffix := range suffixes {
		if strings.HasSuffix(n, suffix) {
			return true
		}
	}
	needles := []string{"API_KEY", "SECRET_KEY", "ACCESS_KEY", "SESSION_TOKEN", "AUTH_TOKEN"}
	for _, needle := range needles {
		if strings.Contains(n, needle) {
			return true
		}
	}
	return false
}
