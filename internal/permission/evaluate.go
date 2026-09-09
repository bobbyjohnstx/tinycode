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
		if WildcardMatch(permission, rule.Permission) && WildcardMatch(pattern, rule.Pattern) {
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

// Disabled returns the set of tool names that are globally denied
// (pattern "*" with action "deny").
func Disabled(tools []string, ruleset Ruleset) map[string]bool {
	result := make(map[string]bool)
	for _, tool := range tools {
		perm := tool
		if editTools[tool] {
			perm = "edit"
		}
		for i := len(ruleset) - 1; i >= 0; i-- {
			rule := ruleset[i]
			if WildcardMatch(perm, rule.Permission) {
				if rule.Pattern == "*" && rule.Action == ActionDeny {
					result[tool] = true
				}
				break
			}
		}
	}
	return result
}
