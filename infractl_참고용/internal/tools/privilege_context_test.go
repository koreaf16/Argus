package tools

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/privilege"
)

type registryPrivilegePrompter struct{}

func (registryPrivilegePrompter) RequestPassword(context.Context, privilege.PromptRequest) (privilege.PromptResponse, error) {
	return privilege.PromptResponse{Password: "secret"}, nil
}

func TestConfigurePrivilegeRegistryWiresPrivilegeAwareTools(t *testing.T) {
	reg := NewRegistry()
	shell := &ShellExecTool{}
	transfer := &FileTransferTool{}
	read := &FileReadTool{}
	write := &FileWriteTool{}
	lineEdit := &FileLineEditTool{}
	sysctl := &SysctlManageTool{}
	for _, tool := range []Tool{shell, transfer, read, write, lineEdit, sysctl} {
		if err := reg.Register(tool); err != nil {
			t.Fatalf("register %s: %v", tool.Name(), err)
		}
	}

	cache := privilege.NewCache()
	handler := registryPrivilegePrompter{}
	ConfigurePrivilegeRegistry(reg, cache, handler)

	if shell.PrivilegeCache != cache || shell.PromptHandler != handler {
		t.Fatalf("shell_exec privilege context not wired")
	}
	if transfer.PrivilegeCache != cache || transfer.PromptHandler != handler {
		t.Fatalf("file_transfer privilege context not wired")
	}
	if read.PrivilegeCache != cache || read.PromptHandler != handler {
		t.Fatalf("file_read privilege context not wired")
	}
	if write.PrivilegeCache != cache || write.PromptHandler != handler {
		t.Fatalf("file_write privilege context not wired")
	}
	if lineEdit.PrivilegeCache != cache || lineEdit.PromptHandler != handler {
		t.Fatalf("file_line_edit privilege context not wired")
	}
	if sysctl.PrivilegeCache != cache || sysctl.PromptHandler != handler {
		t.Fatalf("sysctl_manage privilege context not wired")
	}
}

func TestConfigurePrivilegeToolAllowsHandlerOnlyUpdate(t *testing.T) {
	cache := privilege.NewCache()
	first := registryPrivilegePrompter{}
	shell := &ShellExecTool{PrivilegeCache: cache}
	if !ConfigurePrivilegeTool(shell, nil, first) {
		t.Fatalf("expected shell_exec to be privilege-aware")
	}
	if shell.PrivilegeCache != cache {
		t.Fatalf("nil cache should not clear existing cache")
	}
	if shell.PromptHandler != first {
		t.Fatalf("prompt handler not updated")
	}
}
