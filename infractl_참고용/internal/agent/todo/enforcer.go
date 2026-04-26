// Package todo
// File: enforcer.go
// Description: mutation 도구 호출 시 todo 목록 존재 검증
// Responsibility: Enforce — todo 비어있고 mutation 이면 차단 + 안내 메시지

package todo

// Enforcer 는 TodoStore 를 참조해 mutation 도구 호출 시 todo 선행을 강제한다.
type Enforcer struct {
	store *Store
}

// NewEnforcer 는 Enforcer 를 생성한다.
func NewEnforcer(store *Store) *Enforcer {
	return &Enforcer{store: store}
}

// Enforce 는 mutation 도구(isReadOnly=false) 호출 시 todo 목록이 비어있으면 차단한다.
// (allowed=false, reason 포함) 허용 시 (true, "").
func (e *Enforcer) Enforce(toolName string, isReadOnly bool) (allowed bool, reason string) {
	// TodoWrite, TodoRead 도구는 항상 허용한다.
	if toolName == WriteToolName || toolName == "TodoWrite" || toolName == ReadToolName || toolName == "TodoRead" {
		return true, ""
	}
	// 읽기 전용 도구는 항상 허용한다.
	if isReadOnly {
		return true, ""
	}
	if e.store == nil {
		return true, ""
	}
	items := e.store.List()
	if len(items) == 0 {
		return false, "안전한 작업을 위해 절차 확인이 필요합니다. 복잡한 작업(설치, 배포 등)을 시작하기 전에 TodoWrite 도구를 사용하여 할 일 목록(체크리스트)을 먼저 작성해 주세요. 예: TodoWrite(markdown_list='- [ ] 1. 환경 확인\\n- [ ] 2. 실행')"
	}
	return true, ""
}
