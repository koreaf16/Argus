// Package executor
// File: parallel_format.go
// Description: 병렬 실행 결과 포맷팅
// Responsibility: 다중 서버 실행 결과를 LLM이 소비하기 좋은 구조화된 텍스트로 변환

package executor

import (
	"fmt"
	"strings"
)

// FormatParallelResults는 다중 서버 실행 결과를 구조화된 텍스트로 반환한다.
// 각 서버 결과는 "=== server (Xs) ===" 헤더로 구분된다.
func FormatParallelResults(results []ParallelResult) string {
	if len(results) == 0 {
		return "(실행 결과 없음)"
	}

	var sb strings.Builder
	for i, r := range results {
		if i > 0 {
			sb.WriteByte('\n')
		}
		writeSectionHeader(&sb, r)
		writeContent(&sb, r)
	}
	return sb.String()
}

func writeSectionHeader(sb *strings.Builder, r ParallelResult) {
	if r.Err != nil {
		fmt.Fprintf(sb, "=== %s (오류) ===\n", r.Target)
	} else {
		fmt.Fprintf(sb, "=== %s (%.2fs) ===\n", r.Target, r.Duration.Seconds())
	}
}

func writeContent(sb *strings.Builder, r ParallelResult) {
	if r.Err != nil {
		fmt.Fprintf(sb, "Error: %s\n", r.Err)
		return
	}

	stdout := strings.TrimSpace(r.Result.Stdout)
	stderr := strings.TrimSpace(r.Result.Stderr)

	if stdout != "" {
		sb.WriteString(stdout)
		sb.WriteByte('\n')
	}
	if stderr != "" {
		fmt.Fprintf(sb, "[stderr] %s\n", stderr)
	}
	if stdout == "" && stderr == "" {
		sb.WriteString("(출력 없음)\n")
	}
}
