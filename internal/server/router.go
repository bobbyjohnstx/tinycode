package server

func (s *Server) registerRoutes() {
	// Global
	s.mux.HandleFunc("GET /global/health", s.handleHealth)
	s.mux.HandleFunc("GET /global/version", s.handleVersion)
	s.mux.HandleFunc("GET /global/event", s.handleGlobalEventStream)
	s.mux.HandleFunc("GET /global/config", s.handleConfigGet)
	s.mux.HandleFunc("PATCH /global/config", s.handleConfigUpdate)

	// Instance-scoped event stream
	s.mux.HandleFunc("GET /event", s.handleEventStream)

	// Session CRUD
	s.mux.HandleFunc("POST /session", s.handleSessionCreate)
	s.mux.HandleFunc("GET /session", s.handleSessionList)
	s.mux.HandleFunc("GET /session/status", s.handleSessionStatus)
	s.mux.HandleFunc("GET /session/{id}", s.handleSessionGet)
	s.mux.HandleFunc("PATCH /session/{id}", s.handleSessionUpdate)
	s.mux.HandleFunc("DELETE /session/{id}", s.handleSessionDelete)

	// Session actions
	s.mux.HandleFunc("POST /session/{id}/message", s.handleSessionPrompt)
	s.mux.HandleFunc("POST /session/{sessionID}/prompt_async", s.handleSessionPromptAsync)
	s.mux.HandleFunc("POST /session/{id}/abort", s.handleSessionAbort)
	s.mux.HandleFunc("POST /session/{id}/fork", s.handleSessionFork)
	s.mux.HandleFunc("POST /session/{id}/init", s.handleSessionInit)
	s.mux.HandleFunc("POST /session/{id}/summarize", s.handleSessionSummarize)
	s.mux.HandleFunc("POST /session/{id}/command", s.handleSessionCommand)
	s.mux.HandleFunc("POST /session/{id}/revert", s.handleSessionRevert)
	s.mux.HandleFunc("POST /session/{id}/unrevert", s.handleSessionUnrevert)

	// Session read
	s.mux.HandleFunc("GET /session/{id}/message", s.handleMessageList)
	s.mux.HandleFunc("GET /session/{id}/children", s.handleSessionChildren)
	s.mux.HandleFunc("GET /session/{id}/todo", s.handleSessionTodo)
	s.mux.HandleFunc("GET /session/{id}/diff", s.handleSessionDiff)
	s.mux.HandleFunc("GET /session/{id}/event", s.handleSessionEventStream)

	// Session message mutation
	s.mux.HandleFunc("DELETE /session/{sessionID}/message/{messageID}", s.handleMessageDelete)

	// Session permission
	s.mux.HandleFunc("POST /session/{sessionID}/permissions/{permissionID}", s.handleSessionPermissionReply)

	// Provider
	s.mux.HandleFunc("GET /provider", s.handleProviderList)
	s.mux.HandleFunc("GET /provider/{id}", s.handleProviderGet)
	s.mux.HandleFunc("GET /provider/{id}/model", s.handleModelList)
	s.mux.HandleFunc("GET /provider/{providerID}/model/{modelID}", s.handleModelGet)

	// Permission
	s.mux.HandleFunc("GET /permission", s.handlePermissionList)
	s.mux.HandleFunc("POST /permission/{id}/reply", s.handlePermissionReply)

	// Question
	s.mux.HandleFunc("GET /question", s.handleQuestionList)
	s.mux.HandleFunc("POST /question/{id}/reply", s.handleQuestionReply)

	// Config
	s.mux.HandleFunc("GET /config", s.handleConfigGet)
	s.mux.HandleFunc("PATCH /config", s.handleConfigUpdate)

	// File
	s.mux.HandleFunc("GET /file", s.handleFileList)
	s.mux.HandleFunc("GET /file/content", s.handleFileRead)
	s.mux.HandleFunc("GET /find", s.handleFileSearch)
	s.mux.HandleFunc("GET /find/file", s.handleFileFind)

	// Project
	s.mux.HandleFunc("GET /project", s.handleProjectList)
	s.mux.HandleFunc("GET /project/current", s.handleProjectCurrent)

	// Instance
	s.mux.HandleFunc("GET /path", s.handlePathGet)
	s.mux.HandleFunc("GET /agent", s.handleAgentList)
	s.mux.HandleFunc("GET /skill", s.handleSkillList)
	s.mux.HandleFunc("GET /command", s.handleCommandList)

	// VCS
	s.mux.HandleFunc("GET /vcs", s.handleVCSInfo)
	s.mux.HandleFunc("GET /vcs/status", s.handleVCSStatus)
	s.mux.HandleFunc("GET /vcs/diff", s.handleVCSDiff)
}
