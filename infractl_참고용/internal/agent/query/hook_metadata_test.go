// Package query
// File: hook_metadata_test.go
// Description: ComputeMetadata 단위 테스트
// Responsibility: tool 이름 / shell command 분류 검증

package query

import "testing"

func TestComputeMetadata_ShellReadOnly(t *testing.T) {
	md := ComputeMetadata("bash", map[string]any{"command": "ls -al /tmp"})
	if !md.ReadOnly {
		t.Errorf("ls should be read-only: %+v", md)
	}
	if md.DiskModifying {
		t.Errorf("ls should not be disk-modifying: %+v", md)
	}
}

func TestComputeMetadata_ShellPipe(t *testing.T) {
	md := ComputeMetadata("shell_exec", map[string]any{"command": "cat /etc/hosts | grep 127"})
	if !md.ReadOnly {
		t.Errorf("cat | grep should be read-only: %+v", md)
	}
}

func TestComputeMetadata_PowerShellDirectoryReadOnly(t *testing.T) {
	md := ComputeMetadata("shell_exec", map[string]any{"command": `Get-ChildItem "C:\Users\jhkwa\Downloads" -Name`})
	if !md.ReadOnly {
		t.Errorf("Get-ChildItem should be read-only: %+v", md)
	}
	if md.DiskModifying {
		t.Errorf("Get-ChildItem should not be disk-modifying: %+v", md)
	}
}

func TestComputeMetadata_PowerShellDirAliasReadOnly(t *testing.T) {
	md := ComputeMetadata("shell_exec", map[string]any{"command": `dir C:\Users\jhkwa\Downloads\*oracle* /b`})
	if !md.ReadOnly {
		t.Errorf("dir alias listing should be read-only metadata: %+v", md)
	}
}

func TestComputeMetadata_PowerShellPipelineReadOnly(t *testing.T) {
	md := ComputeMetadata("shell_exec", map[string]any{"command": `Get-ChildItem -LiteralPath "C:\Users\jhkwa\Downloads" -File | Where-Object Name -match "oracle|db_home" | Select-Object Name,Length`})
	if !md.ReadOnly {
		t.Errorf("PowerShell read-only pipeline should be read-only: %+v", md)
	}
	if md.DangerScore != "low" {
		t.Errorf("PowerShell read-only pipeline should be low danger: %+v", md)
	}
}

func TestComputeMetadata_ShellWriteRedirect(t *testing.T) {
	md := ComputeMetadata("bash", map[string]any{"command": "echo hi > /tmp/out"})
	if md.ReadOnly {
		t.Errorf("redirect should not be read-only: %+v", md)
	}
	if !md.DiskModifying {
		t.Errorf("redirect should be disk-modifying: %+v", md)
	}
	// /tmp 경로 쓰기는 시스템 경로가 아니므로 AST 분석 결과 medium
	if md.DangerScore != "medium" {
		t.Errorf("redirect to /tmp should be medium danger: %+v", md)
	}
}

func TestComputeMetadata_ShellDevNullOK(t *testing.T) {
	md := ComputeMetadata("bash", map[string]any{"command": "ls /tmp > /dev/null"})
	if !md.ReadOnly {
		t.Errorf("redirect to /dev/null should still be read-only: %+v", md)
	}
}

func TestComputeMetadata_ShellWriteCommand(t *testing.T) {
	md := ComputeMetadata("bash", map[string]any{"command": "rm -rf /tmp/foo"})
	if md.ReadOnly {
		t.Errorf("rm should not be read-only: %+v", md)
	}
	if !md.DiskModifying {
		t.Errorf("rm should be disk-modifying: %+v", md)
	}
}

func TestComputeMetadata_ShellMissingCommand(t *testing.T) {
	md := ComputeMetadata("bash", map[string]any{})
	if !md.DiskModifying {
		t.Errorf("missing command should default to disk-modifying: %+v", md)
	}
}

func TestComputeMetadata_NetworkTool(t *testing.T) {
	md := ComputeMetadata("http_fetch", map[string]any{"url": "https://example.com"})
	if !md.NetworkAccess {
		t.Errorf("http_fetch should be network: %+v", md)
	}
}

func TestComputeMetadata_DiskTool(t *testing.T) {
	md := ComputeMetadata("file_write", map[string]any{"path": "/tmp/x"})
	if !md.DiskModifying {
		t.Errorf("file_write should be disk-modifying: %+v", md)
	}
}

func TestComputeMetadata_ReadTool(t *testing.T) {
	md := ComputeMetadata("file_read", map[string]any{"path": "/tmp/x"})
	if !md.ReadOnly {
		t.Errorf("file_read should be read-only: %+v", md)
	}
}

func TestComputeMetadata_AskUserQuestion(t *testing.T) {
	md := ComputeMetadata("ask_user_question", map[string]any{"question": "need details"})
	if !md.ReadOnly {
		t.Errorf("ask_user_question should be read-only: %+v", md)
	}
	if md.DiskModifying || md.NetworkAccess {
		t.Errorf("ask_user_question should not look mutating: %+v", md)
	}
}

func TestComputeMetadata_UnknownTool(t *testing.T) {
	md := ComputeMetadata("some_custom_tool", map[string]any{})
	// 모든 플래그가 false 여야 한다
	if md.ReadOnly || md.DiskModifying || md.NetworkAccess {
		t.Errorf("unknown tool should have no flags: %+v", md)
	}
}

func TestComputeMetadata_CommandBase(t *testing.T) {
	// 절대경로 명령 이름 처리: /usr/bin/ls
	md := ComputeMetadata("bash", map[string]any{"command": "/usr/bin/ls /tmp"})
	if !md.ReadOnly {
		t.Errorf("absolute-path ls should be read-only: %+v", md)
	}
}
