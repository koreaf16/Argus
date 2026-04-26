// Package agent
// File: prompt_plan.go
// Description: Plan 모드 시스템 프롬프트 섹션 — 작업 계획 작성 지시
// Responsibility: planModeInstruction()으로 Plan 모드 전용 프롬프트 반환

package agent

// planModeInstruction은 Plan 모드 활성화 시 시스템 프롬프트에 추가할 계획 작성 지시를 반환한다.
// LLM이 read-only 탐색 후 구조화된 한국어 마크다운 계획을 출력하도록 유도한다.
func planModeInstruction() string {
	return `

## Plan Mode (작업 계획 작성 모드)

현재 **Plan Mode**가 활성화되어 있습니다. 코드를 작성하기 전에 반드시 구현 계획을 먼저 작성하세요.

### Plan Mode 규칙
- **탐색만** 허용: Read, Glob, Grep 등 읽기 전용 도구만 사용하세요
- **명령 실행 금지**: 셸 명령, 파일 수정, DB 쿼리를 실행하지 마세요
- **계획 우선**: 코드 작성 없이 설계와 분석에만 집중하세요
- **재사용 발굴**: 기존 함수, 패턴, 인터페이스를 최대한 재사용하세요

### INFRACTL.md 규칙 준수
계획에 다음 규칙이 반드시 반영되어야 합니다:
- 파일당 300라인 제한 (초과 시 파일 분리)
- 모든 파일에 File Header DocBlock 필수
- 단일 책임 원칙(SRP) — 하나의 파일은 하나의 책임
- 모든 I/O의 첫 파라미터는 context.Context
- 에러는 fmt.Errorf("context: %w", err) 로 wrap

### 출력 형식 (한국어 마크다운)

` + "```" + `
## Context
(이 변경이 필요한 이유, 현재 상태, 목표)

## 분석
(관련 파일 경로:라인번호, 기존 패턴, 재사용 가능한 구현)

## 구현 계획
(Step별 변경 사항 — 각 Step에 파일명과 구체적인 코드 포함)

## 검증
(빌드/테스트 명령, 동작 확인 방법)
` + "```" + `

계획이 완성되면 사용자가 승인 후 직접 구현을 진행합니다.
`
}
