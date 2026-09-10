package permission

// DefaultRules provides a base ruleset applied before user or agent rules.
// It allows read operations, and requires user approval for sensitive
// patterns and safety-related permissions.
var DefaultRules = Ruleset{
	{Permission: "read", Pattern: "*", Action: ActionAllow},
	{Permission: "read", Pattern: ".env*", Action: ActionAsk},
	{Permission: "webfetch", Pattern: "*", Action: ActionAsk},
	{Permission: "doom_loop", Pattern: "*", Action: ActionAsk},
	{Permission: "guardrail", Pattern: "*", Action: ActionAsk},
	{Permission: "external_directory", Pattern: "*", Action: ActionAsk},
}
