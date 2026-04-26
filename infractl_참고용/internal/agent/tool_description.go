// Package agent
// File: tool_description.go
// Description: agent 모듈의 기능 수행.
// Responsibility: agent 관련 로직 처리 및 관리.

package agent

import "strings"

func ensureToolDescription(toolName string, args map[string]any) {
	if args == nil || hasToolDescription(args) {
		return
	}
	if desc := inferToolDescription(toolName, args); desc != "" {
		args["description"] = desc
	}
}

func hasToolDescription(args map[string]any) bool {
	for _, key := range []string{"description", "reason", "thought", "explanation", "step", "action"} {
		if v, ok := args[key].(string); ok && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func inferToolDescription(toolName string, args map[string]any) string {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "shell_exec":
		cmd, _ := args["command"].(string)
		return inferShellCommandDescription(cmd)
	case "file_transfer":
		action, _ := args["action"].(string)
		switch strings.ToLower(strings.TrimSpace(action)) {
		case "upload":
			return "파일 업로드"
		case "download":
			return "파일 다운로드"
		default:
			return "파일 전송"
		}
	case "file_read":
		return "파일 또는 디렉터리 확인"
	case "file_write":
		return "파일 내용 변경"
	case "web_search":
		return "관련 자료 검색"
	case "rag_search", "knowledge_search":
		return "저장된 지식 검색"
	default:
		return ""
	}
}

func inferShellCommandDescription(command string) string {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return "명령 실행"
	}

	switch {
	case strings.Contains(lower, "executeprereqs"):
		return "설치 사전조건 검사"
	case strings.Contains(lower, "runinstaller"):
		return "Oracle 설치 프로그램 실행"
	case strings.Contains(lower, "dbca"):
		return "Oracle DBCA 실행"
	case strings.Contains(lower, "oracle-database-preinstall") && (strings.Contains(lower, "rpm -q") || strings.Contains(lower, "dnf list") || strings.Contains(lower, "yum list")):
		return "Oracle 사전 패키지 확인"
	case strings.Contains(lower, "dnf install") || strings.Contains(lower, "yum install") || strings.Contains(lower, "apt install") || strings.Contains(lower, "rpm -i"):
		return "패키지 설치"
	case strings.Contains(lower, "unzip ") || strings.Contains(lower, "tar -"):
		return "설치 미디어 압축 해제"
	case strings.HasPrefix(lower, "tail ") || strings.Contains(lower, " tail "):
		return "로그 확인"
	case strings.HasPrefix(lower, "ps ") || strings.Contains(lower, " pgrep ") || strings.Contains(lower, "process"):
		return "프로세스 확인"
	case strings.HasPrefix(lower, "df ") || strings.Contains(lower, " df "):
		return "디스크 여유공간 확인"
	case strings.HasPrefix(lower, "id ") || strings.Contains(lower, " id ") || strings.Contains(lower, "getent "):
		return "계정과 그룹 확인"
	case strings.HasPrefix(lower, "ls ") || strings.Contains(lower, " find ") || strings.HasPrefix(lower, "find ") || strings.Contains(lower, " test "):
		return "경로 상태 확인"
	default:
		return "명령 실행"
	}
}
