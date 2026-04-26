// Package constants — 턴 완료 메시지 동사 목록.
//
// 파일 역할: 모델 응답이 완료되었을 때 "Worked for 5s" 형태로 표시하는
//
//	과거형 동사 목록을 정의한다.
//
// 포함 모듈:
//   - TurnCompletionVerbs: 과거형 동사 슬라이스.
//
// 호출/사용 방식:
//   - internal/repl/render.go 에서 턴 완료 시간 표시 시 랜덤 동사 선택.
//
// 연결:
//   - 원본: src/constants/turnCompletionVerbs.ts
package constants

// TurnCompletionVerbs 는 턴 완료 시 "for <duration>" 과 결합하는 과거형 동사 목록이다.
var TurnCompletionVerbs = []string{
	"Baked",
	"Brewed",
	"Churned",
	"Cogitated",
	"Cooked",
	"Crunched",
	"Sautéed",
	"Worked",
}
