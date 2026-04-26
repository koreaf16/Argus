// Package constants — 도구 결과 크기 제한.
//
// 파일 역할: 도구 결과를 컨텍스트에 인라인으로 포함할 때의 최대 크기 제한 상수.
//
//	이 한도를 초과하면 결과를 파일로 저장하고 모델에는 미리보기+경로만 전달한다.
//
// 포함 모듈:
//   - DefaultMaxResultSizeChars: 결과 저장 임계 문자 수 (50 000)
//   - MaxToolResultTokens: 토큰 단위 최대 크기 (100 000)
//   - BytesPerToken: 바이트→토큰 변환 추정 계수
//   - MaxToolResultBytes: 바이트 단위 최대 크기
//   - MaxToolResultsPerMessageChars: 단일 사용자 메시지 내 총 한도 (200 000)
//   - ToolSummaryMaxLength: 요약 표시 최대 문자 수 (50)
//
// 호출/사용 방식:
//   - internal/tools/bash, internal/query/engine 등에서 결과 크기 검사 시 참조.
//
// 연결:
//   - 원본: src/constants/toolLimits.ts
package constants

// 결과가 이 문자 수를 초과하면 디스크에 저장하고 모델에는 미리보기만 전달.
const DefaultMaxResultSizeChars = 50_000

// 토큰 단위 도구 결과 최대 크기 (~400 KB 텍스트).
const MaxToolResultTokens = 100_000

// 바이트→토큰 추정 계수 (보수적 추정).
const BytesPerToken = 4

// 바이트 단위 도구 결과 최대 크기 (MaxToolResultTokens × BytesPerToken).
const MaxToolResultBytes = MaxToolResultTokens * BytesPerToken

// 단일 사용자 메시지(한 턴) 내 tool_result 블록 전체 합산 최대 문자 수.
// 병렬 도구 N개가 각각 최대치에 도달해 한 턴에 과도한 컨텍스트를 차지하는 것을 방지.
const MaxToolResultsPerMessageChars = 200_000

// getToolUseSummary() 구현체가 요약 문자열을 자를 최대 문자 수.
const ToolSummaryMaxLength = 50
