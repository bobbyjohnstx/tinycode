package tool

func RegisterBuiltins(r *Registry) {
	r.Register(ReadTool())
	r.Register(WriteTool())
	r.Register(EditTool())
	r.Register(ShellTool())
	r.Register(GrepTool())
	r.Register(GlobTool())
	r.Register(QuestionTool())
	r.Register(WebFetchTool())
}
