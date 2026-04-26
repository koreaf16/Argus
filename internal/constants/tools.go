package constants

const (
	BashToolName      = "bash"
	WebSearchToolName = "web_search"
	TodoWriteToolName = "TodoWrite"

	EnterPlanModeToolName       = "enter_plan_mode"
	ExitPlanModeToolName        = "exit_plan_mode"
	LegacyEnterPlanModeToolName = "EnterPlanMode"
	LegacyExitPlanModeToolName  = "ExitPlanMode"

	AskUserToolName        = "ask_user"
	LegacyAskUserToolName  = "AskUserQuestionTool"
	LegacyAskUserAliasName = "AskUserQuestion"
)

var AllAgentDisallowedTools = map[string]bool{
	ExitPlanModeToolName:        true,
	EnterPlanModeToolName:       true,
	LegacyExitPlanModeToolName:  true,
	LegacyEnterPlanModeToolName: true,
}
