package agent

func (a *Agent) invalidatePromptInputsForTool(toolName string) {
	if a.promptInputs == nil {
		return
	}

	switch toolName {
	case "server_add", "server_remove", "workspace_add", "workspace_remove":
		a.promptInputs.InvalidateServers()
		if a.promptCache != nil {
			a.promptCache.Clear()
		}
	case "save_learned_system":
		a.promptInputs.InvalidateLearnedSystems()
		if a.promptCache != nil {
			a.promptCache.Clear()
		}
	case "rag_register", "knowledge_add":
		a.promptInputs.InvalidateRAG()
		if a.promptCache != nil {
			a.promptCache.Clear()
		}
	}
}
