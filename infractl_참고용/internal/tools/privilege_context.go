// Package tools
// File: privilege_context.go
// Description: tools 모듈의 기능 수행.
// Responsibility: tools 관련 로직 처리 및 관리.

package tools

import "github.com/yourorg/infractl/internal/privilege"

// ConfigurePrivilegeTool attaches shared privilege dependencies to tools that
// support managed sudo/su execution.
func ConfigurePrivilegeTool(tool Tool, cache *privilege.Cache, handler privilege.PromptHandler) bool {
	switch t := tool.(type) {
	case *ShellExecTool:
		if cache != nil {
			t.PrivilegeCache = cache
		}
		t.PromptHandler = handler
		return true
	case *FileTransferTool:
		if cache != nil {
			t.PrivilegeCache = cache
		}
		t.PromptHandler = handler
		return true
	case *FileReadTool:
		if cache != nil {
			t.PrivilegeCache = cache
		}
		t.PromptHandler = handler
		return true
	case *FileWriteTool:
		if cache != nil {
			t.PrivilegeCache = cache
		}
		t.PromptHandler = handler
		return true
	case *FileLineEditTool:
		if cache != nil {
			t.PrivilegeCache = cache
		}
		t.PromptHandler = handler
		return true
	case *SysctlManageTool:
		if cache != nil {
			t.PrivilegeCache = cache
		}
		t.PromptHandler = handler
		return true
	default:
		return false
	}
}

// ConfigurePrivilegeRegistry applies shared privilege dependencies to every
// registered tool that supports them.
func ConfigurePrivilegeRegistry(reg *Registry, cache *privilege.Cache, handler privilege.PromptHandler) {
	if reg == nil {
		return
	}
	for _, tool := range reg.List() {
		ConfigurePrivilegeTool(tool, cache, handler)
	}
}
