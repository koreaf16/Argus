// Package agent
// File: tool_result_limits.go
// Description: 도구별 LLM 전달 결과 최대 바이트 한계 정의
// Responsibility: 도구 종류에 따라 적절한 출력 크기 제한을 반환하여 토큰 예산과 정보 손실을 균형 있게 제어

package agent

// toolResultLimit는 도구 이름에 따라 LLM에 전달할 결과의 최대 바이트 수를 반환한다.
//
// 설계 근거:
//   - network_info, discover_services: 포트 스캔 결과는 고가치 데이터이므로 넉넉히 허용
//   - shell_exec: DB 쿼리·명령 출력이 수만 bytes에 달할 수 있어 보수적 제한
//     → SmartTruncate가 head(헤더/첫 행) + tail(마지막 행/"N rows selected") 보존
//   - log_tail: 로그는 방대할 수 있으므로 더 보수적으로 제한
//   - 그 외: 기본값 16KB (DefaultMaxLLMBytes)
func toolResultLimit(toolName string) int {
	switch toolName {
	case "network_info", "discover_services":
		return 24000
	case "system_info", "process_list":
		return 8000
	case "shell_exec":
		return 6000
	case "log_tail":
		return 8000
	default:
		return 16000
	}
}
