package tool

import "os"

// RegisterBuiltins registers the core tool set that is always available.
func RegisterBuiltins(r *Registry) {
	r.Register(ReadTool())
	r.Register(WriteTool())
	r.Register(EditTool())
	r.Register(ApplyPatchTool())
	r.Register(ShellTool())
	r.Register(GrepTool())
	r.Register(GlobTool())
	r.Register(QuestionTool())
	r.Register(WebFetchTool())
	r.Register(InvalidTool())
	r.Register(TaskTool())
	r.Register(TodoWriteTool())
}

// BuiltinConfig holds optional configuration for conditional tool registration.
type BuiltinConfig struct {
	ConfigDir  string
	ProjectDir string
}

// RegisterConditional registers tools that depend on external configuration
// or API keys. Call after RegisterBuiltins.
func RegisterConditional(r *Registry, cfg BuiltinConfig) {
	if cfg.ConfigDir != "" || cfg.ProjectDir != "" {
		r.Register(SkillTool(cfg.ConfigDir, cfg.ProjectDir))
	}
	if os.Getenv("EXA_API_KEY") != "" {
		r.Register(WebSearchTool())
	}
}
