package tui

func isParallelStatusTool(name string) bool {
	switch name {
	case "discover_services", "discover_web_servers", "system_info", "network_info":
		return true
	default:
		return false
	}
}

func resolveToolBoxContent(
	name string,
	args map[string]any,
	capturedLines []string,
	totalLines int,
	result string,
	success bool,
) ([]string, int) {
	if isShellBoxTool(name) {
		return resolveShellBoxOutput(name, capturedLines, totalLines, result)
	}
	if isParallelStatusTool(name) {
		return capturedLines, totalLines
	}
	return toolBoxContent(name, args, result, success), totalLines
}
