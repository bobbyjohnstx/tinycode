package permission

type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
	ActionAsk   Action = "ask"
)

type Rule struct {
	Permission string `json:"permission"`
	Pattern    string `json:"pattern"`
	Action     Action `json:"action"`
}

type Ruleset []Rule

var editTools = map[string]bool{
	"edit":        true,
	"write":       true,
	"apply_patch": true,
	"todowrite":   true,
}

// Evaluate finds the last matching rule across all rulesets (last-wins semantics).
// DefaultRules are prepended as a base so that user/agent rulesets can override them.
// Returns an "ask" rule if no match is found.
func Evaluate(permission, pattern string, rulesets ...Ruleset) Rule {
	all := make([]Ruleset, 0, len(rulesets)+1)
	all = append(all, DefaultRules)
	all = append(all, rulesets...)
	flat := Merge(all...)
	for i := len(flat) - 1; i >= 0; i-- {
		rule := flat[i]
		if permissionsMatch(permission, rule.Permission) && WildcardMatch(pattern, rule.Pattern) {
			return rule
		}
	}
	return Rule{
		Permission: permission,
		Pattern:    "*",
		Action:     ActionAsk,
	}
}

// Merge concatenates multiple rulesets into a single ruleset.
func Merge(rulesets ...Ruleset) Ruleset {
	var result Ruleset
	for _, rs := range rulesets {
		result = append(result, rs...)
	}
	return result
}

// permissionAliases returns equivalent permission names for matching.
// bash↔shell and list↔glob are treated as aliases on both sides of Evaluate.
func permissionAliases(perm string) []string {
	switch perm {
	case "bash", "shell":
		return []string{"bash", "shell"}
	case "list", "glob":
		return []string{"list", "glob"}
	default:
		return []string{perm}
	}
}

// permissionsMatch reports whether an ask permission matches a rule permission,
// expanding aliases on both sides before WildcardMatch.
func permissionsMatch(permission, rulePermission string) bool {
	for _, p := range permissionAliases(permission) {
		for _, rp := range permissionAliases(rulePermission) {
			if WildcardMatch(p, rp) {
				return true
			}
		}
		// Also match against the raw rule pattern so wildcards like "bas*" still work.
		if WildcardMatch(p, rulePermission) {
			return true
		}
	}
	return false
}

// permissionKeysForTool returns the permission names that should match a tool
// when evaluating Disabled. Edit tools also match "edit"; bash↔shell and
// list↔glob are treated as aliases.
func permissionKeysForTool(tool string) []string {
	keys := permissionAliases(tool)
	if editTools[tool] {
		keys = append(keys, "edit")
	}
	return keys
}

// Disabled returns the set of tool names that are globally denied
// (pattern "*" with action "deny").
func Disabled(tools []string, ruleset Ruleset) map[string]bool {
	result := make(map[string]bool)
	for _, tool := range tools {
		keys := permissionKeysForTool(tool)
		for i := len(ruleset) - 1; i >= 0; i-- {
			rule := ruleset[i]
			matched := false
			for _, key := range keys {
				if permissionsMatch(key, rule.Permission) {
					matched = true
					break
				}
			}
			if matched {
				if rule.Pattern == "*" && rule.Action == ActionDeny {
					result[tool] = true
				}
				break
			}
		}
	}
	return result
}
