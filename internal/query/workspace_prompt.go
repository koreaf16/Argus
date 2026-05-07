package query

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/koreaf16/argus/internal/services/inventory"
	"github.com/koreaf16/argus/internal/services/llm"
	"github.com/koreaf16/argus/internal/services/workspace"
	"github.com/koreaf16/argus/internal/utils"
)

func workspaceSystemBlocks(manager *workspace.Manager) []llm.SystemBlock {
	if manager == nil || manager.Registry() == nil {
		return nil
	}
	entries := manager.Registry().List()
	if len(entries) == 0 {
		return nil
	}

	active := manager.ActiveAlias()
	lines := []string{"사용 가능한 워크스페이스 (등록된 별칭):"}
	remoteCount := 0
	hasDisabledElevation := false
	for _, entry := range entries {
		if entry.Kind == workspace.ServerKindLocal {
			label := "로컬 (kind=local)"
			if entry.Alias == active {
				label += " [활성]"
			}
			lines = append(lines, "- "+label)
			continue
		}

		remoteCount++
		desc := fmt.Sprintf("%s (kind=%s, user=%s, host=%s, port=%d)",
			entry.Alias,
			entry.Kind,
			strings.TrimSpace(entry.User),
			strings.TrimSpace(entry.Host),
			entry.Port,
		)
		if entry.Alias == active {
			desc += " [활성]"
		}
		lines = append(lines, "- "+desc)
		// Elevation status line
		e := entry.Elevation
		if e.Allowed && e.Mode != "none" {
			targets := strings.Join(e.TargetUsers, ", ")
			if targets == "" {
				targets = "모두"
			}
			lines = append(lines, fmt.Sprintf("               권한 상승(elevation): 활성화됨(ENABLED)   mode=%s   targets=%s", e.Mode, targets))
		} else {
			lines = append(lines, fmt.Sprintf("               권한 상승(elevation): 비활성화됨(DISABLED)  (sudo/su가 거부됩니다)"))
			hasDisabledElevation = true
		}
	}
	if remoteCount >= 2 {
		lines = append(lines, "For server_copy, provide explicit source and destination aliases. Prefer alias:path for both endpoints, for example local:docs/README.md -> sandbox-server:/tmp/README.md; alternatively set both src_server and dst_server.")
		lines = append(lines, "다중 워크스페이스 안전 규칙: 두 개 이상의 원격 워크스페이스가 등록되어 있습니다. 쉘/파일/검색/검사/메트릭/계정-쉘 도구에서 반드시 `server`를 명시적으로 설정해야 합니다. `server` 없이 호출하면 거부됩니다.")
		lines = append(lines, "활성 워크플로우가 역할 프로필을 정의하는 경우, 대신 `role`/`channel`을 설정할 수 있습니다. 역할은 정확히 하나의 서버로 해석되어야 하며 일치하지 않는 서버+역할 호출은 거부됩니다.")
		lines = append(lines, "‘server_copy’의 경우, 반드시 소스 및 대상 별칭을 모두 명시적으로 제공해야 합니다 (양쪽에 alias:path를 사용하거나 `src_server` 및 `dst_server`를 모두 사용).")
	}
	lines = append(lines, "응답에 저장된 비밀번호 값을 포함하거나 요청하지 마십시오.")

	// Elevation policy section (only added when there are remote servers)
	if remoteCount > 0 {
		lines = append(lines, elevationPolicyPrompt(hasDisabledElevation))
	}

	blocks := []llm.SystemBlock{
		{Type: "text", Text: strings.Join(lines, "\n")},
	}

	if envBlock := buildActiveEnvBlock(manager, active, remoteCount >= 2); envBlock != "" {
		blocks = append(blocks, llm.SystemBlock{Type: "text", Text: envBlock})
	}

	return blocks
}

func buildActiveEnvBlock(manager *workspace.Manager, active string, strictMultiWorkspace bool) string {
	entry, ok := manager.Registry().Get(active)
	if !ok {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "활성 환경(Active environment):\n")
	fmt.Fprintf(&sb, "  워크스페이스: %s\n", active)

	if entry.Kind == workspace.ServerKindLocal {
		fmt.Fprintf(&sb, "  유형(kind): 로컬(local)\n")
		fmt.Fprintf(&sb, "  os: %s/%s\n", runtime.GOOS, runtime.GOARCH)

		if runtime.GOOS == "windows" {
			fmt.Fprintf(&sb, "  선호하는 쉘 도구: powershell\n")
			if sh, err := utils.FindSuitableShell(); err == nil && sh != "" {
				fmt.Fprintf(&sb, "  bash 호환 쉘: %s\n", sh)
			}
		} else if sh, err := utils.FindSuitableShell(); err == nil && sh != "" {
			fmt.Fprintf(&sb, "  선호하는 쉘 도구: bash\n")
			fmt.Fprintf(&sb, "  쉘: %s\n", sh)
		}

		if user := os.Getenv("USERNAME"); user != "" {
			fmt.Fprintf(&sb, "  사용자: %s\n", user)
		} else if user := os.Getenv("USER"); user != "" {
			fmt.Fprintf(&sb, "  사용자: %s\n", user)
		}

		sb.WriteString("\n동작 규칙:\n")
		if runtime.GOOS == "windows" {
			sb.WriteString("  - 활성 OS는 Windows입니다. 로컬 시스템 작업에는 'powershell' 도구를 사용하십시오.\n")
			sb.WriteString("  - 로컬 Windows 명령에 'bash'를 사용하지 마십시오. 'bash'는 도구 호출이 `server` 또는 `role`을 통해 유닉스 계열 원격 워크스페이스를 명시적으로 대상으로 하는 경우에만 사용하십시오.\n")
			sb.WriteString("  - 리눅스 전용 명령(예: apt, yum, systemctl)을 내리지 마십시오.\n")
		} else {
			sb.WriteString("  - 활성 OS는 유닉스 계열입니다. 로컬 시스템 작업에는 'bash' 도구를 사용하십시오.\n")
			sb.WriteString("  - 로컬 유닉스 계열 명령에 'powershell'을 사용하지 마십시오. 'powershell'은 도구 호출이 `server` 또는 `role`을 통해 Windows 워크스페이스를 명시적으로 대상으로 하는 경우에만 사용하십시오.\n")
		}
		sb.WriteString("  - `server` 매개변수를 지정하지 않는 한 쉘 도구는 로컬 머신에서 실행됩니다.\n")
		return sb.String()
	}

	// Remote (SSH)
	fmt.Fprintf(&sb, "  유형(kind): ssh\n")
	fmt.Fprintf(&sb, "  호스트: %s@%s:%d\n",
		strings.TrimSpace(entry.User),
		strings.TrimSpace(entry.Host),
		entry.Port,
	)

	invSnap, hasSnap := manager.GetInventorySnapshot(active)
	if !hasSnap || invSnap.System == nil {
		sb.WriteString("  환경: 아직 수집되지 않음 — 서버 연결 후 인벤토리가 자동 수집됩니다\n")
		sb.WriteString("\n동작 규칙:\n")
		if strictMultiWorkspace {
			sb.WriteString("  - 여러 원격 워크스페이스가 등록되어 있습니다. 각 쉘/파일/검색/검사/메트릭 호출 시 반드시 `server`를 명시적으로 설정해야 합니다.\n")
		} else {
			sb.WriteString("  - 쉘 도구는 기본적으로 원격 서버에서 실행됩니다 (`server` 매개변수 불필요).\n")
		}
		sb.WriteString("  - 로컬 머신에서 명령을 실행하려면 도구 호출 시 server=\"local\"을 지정하십시오.\n")
		return sb.String()
	}

	sys := invSnap.System
	if sys.OSPretty != "" {
		fmt.Fprintf(&sb, "  os: %s\n", sys.OSPretty)
	} else if sys.OS != "" {
		fmt.Fprintf(&sb, "  os: %s\n", firstLine(sys.OS))
	}
	if sys.Shell != "" {
		fmt.Fprintf(&sb, "  쉘: %s\n", firstLine(sys.Shell))
	}
	if sys.User != "" {
		fmt.Fprintf(&sb, "  사용자: %s\n", sys.User)
	}
	if sys.CWD != "" {
		fmt.Fprintf(&sb, "  현재 디렉토리(cwd): %s\n", sys.CWD)
	}
	if sys.Uptime != "" {
		fmt.Fprintf(&sb, "  업타임: %s\n", sys.Uptime)
	}
	if sys.Memory != "" {
		fmt.Fprintf(&sb, "  메모리:\n%s\n", indent(sys.Memory, "    "))
	}
	if sys.ListenerCount > 0 {
		fmt.Fprintf(&sb, "  리스닝 포트 수: %d\n", sys.ListenerCount)
	}
	if sys.ServiceCount > 0 {
		fmt.Fprintf(&sb, "  실행 중인 서비스 수: %d\n", sys.ServiceCount)
	}
	if len(invSnap.Containers) > 0 {
		fmt.Fprintf(&sb, "  도커 컨테이너: %s\n", formatContainerSummary(invSnap.Containers))
	}

	sb.WriteString("\n동작 규칙:\n")
	if strings.Contains(strings.ToLower(sys.OS), "windows") {
		sb.WriteString("  - 활성 OS는 Windows입니다. 이 워크스페이스에는 'powershell' 도구를 사용하십시오.\n")
		sb.WriteString("  - 이 Windows 워크스페이스에 'bash'를 사용하지 마십시오. 'bash'는 다른 유닉스 계열 워크스페이스를 명시적으로 대상으로 하는 경우에만 사용하십시오.\n")
	} else {
		sb.WriteString("  - 활성 OS는 유닉스 계열입니다. 이 워크스페이스에는 'bash' 도구를 사용하십시오.\n")
		sb.WriteString("  - 이 유닉스 계열 워크스페이스에 'powershell'을 사용하지 마십시오. 'powershell'은 다른 Windows 워크스페이스를 명시적으로 대상으로 하는 경우에만 사용하십시오.\n")
	}
	if strictMultiWorkspace {
		sb.WriteString("  - 여러 원격 워크스페이스가 등록되어 있습니다. 각 쉘/파일/검색/검사/메트릭 호출 시 반드시 `server`를 명시적으로 설정해야 합니다.\n")
	} else {
		sb.WriteString("  - 쉘 도구는 기본적으로 원격 서버에서 실행됩니다 (`server` 매개변수 불필요).\n")
	}
	sb.WriteString("  - 로컬 머신에서 명령을 실행하려면 도구 호출 시 server=\"local\"을 지정하십시오.\n")
	sb.WriteString("  - sudo/su 명령을 내리기 전에 워크스페이스 목록에서 '권한 상승(elevation)' 라인을 확인하십시오.\n")
	sb.WriteString("    이 서버에 대해 권한 상승이 DISABLED인 경우, 중지하고 사용자를 /server edit <alias>로 안내하십시오.\n")

	return sb.String()
}

func formatContainerSummary(containers []inventory.ContainerInfo) string {
	total := len(containers)
	running := 0
	var names []string
	for _, c := range containers {
		if c.State == "running" {
			running++
			names = append(names, c.Name)
		}
	}
	summary := fmt.Sprintf("%d/%d running", running, total)
	if running > 0 && len(names) <= 3 {
		summary += ": " + strings.Join(names, ", ")
	}
	return summary
}

// elevationPolicyPrompt returns the LLM instruction block that explains how
// to interpret the per-server elevation lines and what to do before calling
// bash with a sudo/su command.
func elevationPolicyPrompt(anyDisabled bool) string {
	sb := &strings.Builder{}
	sb.WriteString("\n권한 상승 정책 (ELEVATION POLICY - 서버별 강제 적용 — 시도 전 중단 및 확인):\n")
	sb.WriteString("각 서버에는 고유한 권한 상승 정책이 있습니다. sudo/su 명령을 실행하기 전에\n")
	sb.WriteString("위의 별칭별 권한 상승 라인을 확인하고 결정하십시오:\n\n")

	sb.WriteString("  Case 1) 권한 상승(elevation): 비활성화됨(DISABLED)\n")
	sb.WriteString("    ► 중단하십시오. 어떤 도구도 호출하지 마십시오. \"어떻게 되는지 보기 위해\"\n")
	sb.WriteString("      sudo 명령을 시도하지 마십시오. 사용자에게 한국어로 다음과 같이 응답하십시오:\n")
	sb.WriteString("        - 어떤 명령에 루트 권한이 필요했는지 (명령 인용)\n")
	sb.WriteString("        - 왜 중단했는지 (서버의 권한 상승 정책이 OFF임)\n")
	sb.WriteString("        - 활성화 방법: \"`/server edit <alias>` 명령으로 elevation을\n")
	sb.WriteString("          켜고 sudo 비밀번호를 등록하신 뒤 다시 요청해 주세요\"\n")
	sb.WriteString("      턴을 종료하십시오. 사용자가 정책을 등록한 후 다시 요청할 때까지 재시도하지 마십시오.\n\n")

	sb.WriteString("  Case 2) 권한 상승(elevation): 활성화됨(ENABLED), 대상 사용자가 targets에 있음 (또는 targets=모두)\n")
	sb.WriteString("    ► 정상적으로 진행하십시오. 채널이 자격 증명 저장소(mode=password)의 sudo 비밀번호 또는\n")
	sb.WriteString("      SSH 로그인 비밀번호(mode=reuse_login)를 자동으로 주입합니다.\n")
	sb.WriteString("      사용자에게 비밀번호를 묻지 마십시오.\n\n")

	sb.WriteString("  Case 3) 권한 상승(elevation): 활성화됨(ENABLED)이지만 요청된 대상 사용자가 targets에 없음\n")
	sb.WriteString("    ► 중단하십시오. 사용자에게 한국어로 어떤 대상 사용자가 요청되었으며\n")
	sb.WriteString("      허용 목록을 벗어났음을 알리십시오. /server edit <alias>를 제안하십시오.\n")
	sb.WriteString("      도구를 호출하지 마십시오.\n\n")

	sb.WriteString("- 사용자에게 sudo/su 비밀번호를 직접 묻지 마십시오. 비밀번호는\n")
	sb.WriteString("  `/server edit <alias>`를 통해 등록됩니다.\n")
	sb.WriteString("- 채널 라우터에는 안전망으로서 자체 거부 계층이 있습니다. 만약 여기에\n")
	sb.WriteString("  걸린다면, 당신의 추론에 오류가 있는 것으로 간주하십시오. 위의 정책에 따라\n")
	sb.WriteString("  더 일찍 중단했어야 합니다.")

	if anyDisabled {
		sb.WriteString("\n\n중요: 하나 이상의 서버에서 권한 상승이 비활성화됨(DISABLED)으로 설정되어 있습니다.\n")
		sb.WriteString("해당 서버의 경우 sudo/su 시도 전에 반드시 중단하고 사용자를 /server edit <alias>로\n")
		sb.WriteString("안내해야 합니다. 이는 제안이 아닌 강제 사항입니다.\n")
		sb.WriteString("마이그레이션 참고: 명시적 정책 없이 sudo를 사용하던 기존 설정인 경우, 사용자는\n")
		sb.WriteString("환경 변수에 ARGUS_ELEVATION_LEGACY=1을 설정하여 임시로 (/server edit <alias>를 통해\n")
		sb.WriteString("명시적 정책을 등록하는 동안 모든 서버에 대해 reuse_login 적용) 폴백으로 사용할 수 있습니다.")
	}

	return sb.String()
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}

func indent(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

func truncateLines(s string, max int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= max {
		return s
	}
	return strings.Join(lines[:max], "\n") + fmt.Sprintf("\n... (%d more lines)", len(lines)-max)
}
