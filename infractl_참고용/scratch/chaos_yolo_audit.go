//go:build tools
// +build tools

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
	"github.com/yourorg/infractl/internal/checkpoint"
)

type yoloPrompter struct {
	st store.ServerStore
}

func (p *yoloPrompter) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	srv, _ := p.st.Get(ctx, req.Target)
	return privilege.PromptResponse{Password: srv.Credential}, nil
}

func (p *yoloPrompter) RequestQuestion(ctx context.Context, req tools.QuestionRequest) (tools.QuestionResponse, error) {
	// Should not be called in YOLO mode, but just in case
	return tools.QuestionResponse{SelectedIndex: 0, SelectedLabel: "Run Anyway"}, nil
}

func main() {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(ctx, ".infractl/infractl.db")
	if err != nil { panic(err) }
	defer st.Close()

	srv, _ := st.Get(ctx, "sandbox")
	client := sshconn.NewClient(&sshconn.Config{
		Host: srv.Host, Port: srv.Port, User: srv.User, Password: srv.Credential, WorkspaceDir: srv.WorkspaceDir,
		AuthType: string(srv.AuthType),
	})
	defer client.Close()
	exec := sshconn.NewSSHExecutor(srv.Name, client, srv.OS, srv.WorkspaceDir)

	privCache := privilege.NewCache()
	apprCache := tools.NewApprovalCache()
	cpMgr := checkpoint.NewManager(st)
	prompter := &yoloPrompter{st: st}

	// YOLO 모드 시뮬레이션: 무조건 true 반환
	yoloModeFunc := func() bool { return true }

	shellTool := &tools.ShellExecTool{
		PrivilegeCache: privCache, ApprovalCache: apprCache, IsYoloMode: yoloModeFunc, PromptHandler: prompter,
	}
	writeTool := &tools.FileWriteTool{
		PrivilegeCache: privCache, ApprovalCache: apprCache, CheckpointManager: cpMgr, PromptHandler: prompter,
	}
	sysctlTool := &tools.SysctlManageTool{
		PrivilegeCache: privCache, ApprovalCache: apprCache, CheckpointManager: cpMgr, PromptHandler: prompter,
	}

	// YOLO 모드 흉내: 모든 위험 작업을 1시간 동안 무조건 승인 처리 (프롬프트 바이패스)
	apprCache.Grant(srv.Name, "global_risk", 1*time.Hour)
	apprCache.Grant(srv.Name, "sysctl", 1*time.Hour)

	fmt.Println("☠️ !!! STARTING ULTIMATE YOLO CHAOS AUDIT !!! ☠️")

	// --- [Target 1: Ruthless Deletion] ---
	fmt.Println("\n[Target 1] Executing destructive 'rm -rf /tmp/yolo_dummy' without asking...")
	exec.Execute(ctx, "mkdir -p /tmp/yolo_dummy")
	out, _ := shellTool.Execute(ctx, map[string]interface{}{
		"command": "rm -rf /tmp/yolo_dummy",
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)
	checkOut, _ := exec.Execute(ctx, "ls -d /tmp/yolo_dummy")
	if checkOut.ExitCode != 0 {
		fmt.Println("💥 SUCCESS: Directory destroyed instantly without prompts.")
	}

	// --- [Target 2: Kernel Manipulation] ---
	fmt.Println("\n[Target 2] Manipulating Kernel (vm.drop_caches=3) instantly...")
	out, _ = sysctlTool.Execute(ctx, map[string]interface{}{
		"key": "vm.drop_caches", "value": "3", "persist": false, "become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)

	// --- [Target 3: The Suicide Attempt (Syntax Validation Test)] ---
	fmt.Println("\n[Target 3] Sabotaging /etc/sudoers with garbage data (EXPECTED TO FAIL)...")
	// 이 작업은 YOLO 모드(캐시 승인)라 할지라도 ValidateConfig 로직에 의해 차단되어야 합니다.
	out, _ = writeTool.Execute(ctx, map[string]interface{}{
		"path": "/etc/sudoers", 
		"content": "this_is_garbage_data_that_will_break_sudo\n", 
		"append": true, 
		"become_method": "sudo",
	}, exec)
	fmt.Println("Result:", out.Content)
	
	if strings.Contains(out.Content, "[Validation Error]") {
		fmt.Println("🛡️ SAVED: The system survived! Syntax validation blocked the fatal typo even in YOLO mode.")
	} else {
		fmt.Println("💀 FATAL: The system allowed writing garbage to sudoers. We might be locked out of sudo!")
	}

	fmt.Println("\n☠️ YOLO AUDIT COMPLETE. ☠️")
}
