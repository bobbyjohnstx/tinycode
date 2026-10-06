package permission

// DefaultRules provides a base ruleset applied before user or agent rules.
// It allows read operations, and requires user approval for sensitive
// patterns and external directory access.
//
// Note: doom_loop is enforced as a hard-stop in the session processor
// (checkDoomLoop), not via permission.Ask. Destructive shell commands use
// the "destructive-shell" permission from the shell/monitor tools.
var DefaultRules = Ruleset{
	{Permission: "read", Pattern: "*", Action: ActionAllow},
	{Permission: "read", Pattern: ".env*", Action: ActionAsk},
	{Permission: "webfetch", Pattern: "*", Action: ActionAsk},
	{Permission: "external_directory", Pattern: "*", Action: ActionAsk},
}
