// Package agent
// File: intel.go
// Description: Reasoning-tier 작업 전 인텔 서브에이전트 실행 및 결과 주입 오케스트레이터
// Responsibility: TaskType 분류, 인텔 서브에이전트 호출, 결과를 시스템 프롬프트 섹션으로 변환

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yourorg/infractl/internal/subagent"
)

// TaskType은 reasoning-tier 작업의 유형을 나타낸다.
type TaskType string

const (
	TaskTypeInstall      TaskType = "install"      // 소프트웨어 설치 / 배포
	TaskTypeTroubleshoot TaskType = "troubleshoot" // 트러블슈팅 / 디버그
	TaskTypeConfigure    TaskType = "configure"    // 설정 / 구성
	TaskTypeAnalyze      TaskType = "analyze"      // 분석 / 점검
	TaskTypeMigrate      TaskType = "migrate"      // 마이그레이션 / 업그레이드
	TaskTypePlan         TaskType = "plan"         // 계획 / 아키텍처 설계
)

// buildIntelQuestion은 인텔 서브에이전트에 전달할 조사 질문을 생성한다.
// activeServer가 비어 있지 않으면 해당 워크스페이스를 파일 탐색 우선 대상으로 지시한다.
func buildIntelQuestion(taskType TaskType, userInput, activeServer string) string {
	switch taskType {
	case TaskTypeInstall:
		fileSearchGuide := buildInstallFileSearchGuide(activeServer)
		return fmt.Sprintf(
			"사용자가 다음 설치 작업을 요청했습니다: [%s]\n\n"+
				"설치 실행 전에 **두 단계를 반드시 순서대로 완료**하세요.\n\n"+

				"## Phase 1: 환경 체크 (로컬/서버 명령 실행)\n"+
				"아래 항목을 셸 명령으로 확인하세요:\n"+
				"1. system_info 도구로 OS 버전·배포판·아키텍처 확인\n"+
				"2. disk_usage 도구로 /tmp, /opt, /var 등 주요 경로의 가용 공간 확인\n"+
				"3. shell_exec 도구로 현재 사용자 및 sudo 권한 확인 (두 번 병렬 호출):\n"+
				"   - 기본: command=`whoami && id`\n"+
				"   - sudo 확인: command=`id`, become_method=`sudo` (저장된 SSH 비밀번호 자동 사용; 성공=SUDO_OK, 실패=SUDO_FAIL)\n"+
				"4. shell_exec 도구로 동일 소프트웨어의 기존 설치 여부 확인\n"+
				"   (rpm -qa, dpkg -l, which, systemctl status 등 OS에 맞게 선택)\n"+
				"5. shell_exec 도구로 패키지 매니저(yum/dnf/apt/pip/npm 등) 가용 여부 확인\n"+
				"%s\n"+
				"## Phase 2: 웹 리서치 (반드시 web_search 도구로 수행)\n"+
				"Phase 1에서 확인한 OS 버전을 사용하여 **아래 4가지 쿼리를 모두 검색**하세요:\n"+
				"1. '[소프트웨어] install [OS 버전] site:docs OR site:github OR site:official' — 공식 설치 가이드\n"+
				"2. '[소프트웨어] latest release version [현재연도]' — 최신 릴리스 버전 및 변경 사항\n"+
				"3. '[소프트웨어] [OS 버전] prerequisites requirements dependencies' — 사전 요구사항\n"+
				"4. '[소프트웨어] [OS 버전] known issues installation error' — 알려진 설치 문제\n"+
				"검색 결과가 불충분하면 추가 쿼리를 사용하세요. web_search를 건너뛰면 안 됩니다.\n\n"+

				"두 단계를 모두 완료한 후 수집된 정보를 보고하세요.",
			userInput, fileSearchGuide,
		)
	case TaskTypeTroubleshoot:
		return fmt.Sprintf(
			"사용자가 다음 문제를 보고했습니다: [%s]\n\n"+
				"문제 해결에 앞서 현재 시스템 상태를 파악하세요. 특히:\n"+
				"- 현재 시스템 환경 (OS, 메모리, 디스크)\n"+
				"- 관련 서비스/프로세스의 현재 상태\n"+
				"- 관련 로그의 최근 에러 메시지\n"+
				"- web_search 도구로 에러 메시지 또는 증상을 검색하여 알려진 해결 방법 확인\n"+
				"- rag_search 도구로 이전에 학습한 유사 문제 및 해결 사례 확인\n"+
				"- 최근 변경 사항이 있었는지 확인",
			userInput,
		)
	case TaskTypeConfigure:
		return fmt.Sprintf(
			"사용자가 다음 설정 변경을 요청했습니다: [%s]\n\n"+
				"설정 변경 전에 현재 상태를 파악하세요. 특히:\n"+
				"- 현재 시스템 환경\n"+
				"- 변경 대상의 현재 설정값\n"+
				"- 관련 설정 파일 위치\n"+
				"- 변경 시 영향받는 다른 서비스나 설정\n"+
				"- 공식 권장 설정값",
			userInput,
		)
	case TaskTypeAnalyze:
		return fmt.Sprintf(
			"사용자가 다음 분석을 요청했습니다: [%s]\n\n"+
				"분석에 필요한 기초 정보를 수집하세요. 특히:\n"+
				"- 현재 시스템 자원 사용 현황 (CPU, 메모리, 디스크)\n"+
				"- 관련 서비스와 프로세스 목록\n"+
				"- 분석 대상과 관련된 기존 지식\n"+
				"- 분석에 활용할 수 있는 도구와 방법",
			userInput,
		)
	case TaskTypeMigrate:
		return fmt.Sprintf(
			"사용자가 다음 마이그레이션을 요청했습니다: [%s]\n\n"+
				"마이그레이션 전에 현재 상태를 파악하세요. 특히:\n"+
				"- 현재 시스템 환경과 버전\n"+
				"- 디스크 가용 공간\n"+
				"- 영향받는 서비스 목록\n"+
				"- 마이그레이션 방법과 주의사항\n"+
				"- 롤백 방법",
			userInput,
		)
	default: // TaskTypePlan 및 기타
		return fmt.Sprintf(
			"사용자가 다음 계획 수립을 요청했습니다: [%s]\n\n"+
				"계획 수립에 필요한 정보를 수집하세요. 특히:\n"+
				"- 현재 시스템 환경\n"+
				"- 관련 서비스와 인프라 현황\n"+
				"- 관련 기술 문서와 베스트 프랙티스\n"+
				"- 기존에 학습된 관련 지식",
			userInput,
		)
	}
}

// runIntelSubagent는 인텔 서브에이전트를 실행하여 환경 조사 리포트를 반환한다.
// 서브에이전트가 없거나 실패하면 빈 문자열과 오류를 반환한다.
func runIntelSubagent(ctx context.Context, runner *subagent.Runner, taskType TaskType, userInput, server string) (string, error) {
	question := buildIntelQuestion(taskType, userInput, server)
	cfg := subagent.SubagentConfig{
		Type:     subagent.AgentTypeIntel,
		Server:   server,
		Question: question,
	}

	slog.Info("intel subagent start", "task_type", taskType, "server", server)
	result := runner.Execute(ctx, cfg)
	if result.Err != nil {
		return "", fmt.Errorf("intel subagent: %w", result.Err)
	}

	slog.Info("intel subagent done",
		"task_type", taskType,
		"input_tokens", result.InputTokens,
		"output_tokens", result.OutputTokens,
		"tool_uses", result.ToolUses,
	)
	return result.Answer, nil
}

// formatIntelPromptSection은 인텔 리포트와 작업 계획 지침을 시스템 프롬프트 섹션으로 포맷한다.
// yoroMode가 true면 환경 정보만 주입하고 사용자 승인 요청 지침은 생략한다.
func formatIntelPromptSection(intelReport string, taskType TaskType, yoroMode bool) string {
	var sb strings.Builder

	sb.WriteString("\n\n## 사전 환경 조사 보고서\n")
	sb.WriteString("*(작업 실행 전 자동 수집된 환경 정보)*\n\n")
	sb.WriteString(strings.TrimSpace(intelReport))
	sb.WriteString("\n\n")

	if yoroMode {
		// YORO 모드: 환경 정보만 활용, 승인 요청·질문 없이 바로 실행
		sb.WriteString("## 작업 지침 (YORO 모드)\n")
		sb.WriteString("위 환경 정보를 바탕으로 최적의 방법을 선택하여 **사용자에게 계획을 설명하거나 승인을 묻지 말고 즉시 Tool(도구)을 호출하여 실행**하세요.\n\n")
		sb.WriteString("**엄격히 준수:**\n")
		sb.WriteString("- `ask_user_question` 도구 호출 금지 — 질문 없이 최선의 판단으로 진행\n")
		sb.WriteString("- 사용자 확인·승인 요청 금지\n")
		sb.WriteString("- \"진행할까요?\", \"진행하겠습니다\" 등 확인성/선언성 발화 최소화 또는 생략\n")
		sb.WriteString("- **텍스트 응답보다 도구 호출(Tool Call)을 우선**하여 흐름이 끊기지 않게 함\n\n")
		sb.WriteString("판단이 필요한 선택지가 있으면 가장 안전하고 표준적인 옵션을 자동 선택하세요.\n")
	} else {
		sb.WriteString("## 작업 계획 수립 지침\n")
		sb.WriteString("위 환경 정보를 바탕으로 **반드시 아래 순서**를 따르세요:\n\n")
		sb.WriteString("1. **환경 요약** — 수집된 정보를 사용자에게 간결하게 제시\n")
		sb.WriteString("2. **작업 계획** — 구체적인 실행 계획 (방법, 위치, 버전, 옵션 포함)\n")
		sb.WriteString("3. **사용자 확인** — `ask_user_question` 도구로 계획 승인 요청\n")
		sb.WriteString("4. **실행** — 승인 후에만 실행\n\n")
		sb.WriteString(intelPlanInstruction(taskType))
	}

	return sb.String()
}

// intelPlanInstruction은 작업 유형별 계획 작성 추가 지침을 반환한다.
func intelPlanInstruction(taskType TaskType) string {
	base := "**반드시 `ask_user_question` 도구를 사용하여 사용자에게 계획을 보여주고 승인을 받은 후에만 실행하세요.**\n"

	switch taskType {
	case TaskTypeInstall:
		return "### 설치 계획에 포함할 항목\n" +
			"- 설치 방법 (패키지 매니저 / 소스 빌드 / 바이너리)\n" +
			"- 설치할 버전\n" +
			"- 설치 위치 및 설정 파일 경로\n" +
			"- 예상 디스크 사용량\n" +
			"- 기존 환경에 미치는 영향 (포트 충돌, 서비스 재시작 등)\n" +
			"- 추가 옵션 또는 사용자 선택 사항\n\n" + base
	case TaskTypeTroubleshoot:
		return "### 트러블슈팅 계획에 포함할 항목\n" +
			"- 현재 증상 요약\n" +
			"- 확인할 항목 목록 (단계별)\n" +
			"- 예상 원인 (우선순위 순)\n" +
			"- 각 원인에 대한 조치 방법\n" +
			"- 조치 중 주의사항\n\n" + base
	case TaskTypeConfigure:
		return "### 설정 변경 계획에 포함할 항목\n" +
			"- 변경 대상 파일 및 설정 키\n" +
			"- 현재 값 → 변경할 값\n" +
			"- 변경이 필요한 이유\n" +
			"- 영향받는 서비스 및 재시작 필요 여부\n\n" + base
	case TaskTypeAnalyze:
		return "### 분석 계획에 포함할 항목\n" +
			"- 분석 범위 및 목표\n" +
			"- 수집할 메트릭과 방법\n" +
			"- 판단 기준 및 임계값\n" +
			"- 분석 결과 제공 형식\n\n" + base
	case TaskTypeMigrate:
		return "### 마이그레이션 계획에 포함할 항목\n" +
			"- 현재 상태 → 목표 상태\n" +
			"- 마이그레이션 단계별 순서\n" +
			"- 다운타임 예상 여부\n" +
			"- 롤백 계획\n" +
			"- 검증 방법\n\n" + base
	default: // TaskTypePlan
		return "### 계획 작성 시 포함할 항목\n" +
			"- 현재 아키텍처 / 상태\n" +
			"- 목표 아키텍처 / 상태\n" +
			"- 전환 단계별 계획\n" +
			"- 리스크 및 완화 방안\n\n" + base
	}
}

// buildInstallFileSearchGuide는 설치 파일 탐색 지침을 activeServer 기준으로 생성한다.
func buildInstallFileSearchGuide(activeServer string) string {
	if activeServer != "" {
		return fmt.Sprintf(
			"6. 사용자가 설치 파일이 있다고 언급하면 먼저 **%s** 파일시스템에서 탐색하세요.\n"+
				"   (find 또는 ls로 특정 키워드 필터 없이 전체 목록 조회)\n"+
				"   소프트웨어 설치 파일은 제품명이 파일명에 없는 경우가 많습니다\n"+
				"   (예: Oracle → LINUX.X64_193000_db_home.zip, OPatch → p6880880_*.zip).\n"+
				"   사용자가 infractl 실행 머신의 경로를 명시적으로 언급한 경우에만 target=localhost로 추가 조회하세요.\n\n",
			activeServer,
		)
	}
	return "6. 사용자가 설치 파일 경로를 언급하면 해당 디렉터리의 **모든 파일**을\n" +
		"   shell_exec(target=localhost)로 조회하세요. 특정 키워드 필터를 사용하지 마세요.\n" +
		"   소프트웨어 설치 파일은 제품명이 파일명에 없는 경우가 많습니다\n" +
		"   (예: Oracle → LINUX.X64_193000_db_home.zip, OPatch → p6880880_*.zip).\n\n"
}
