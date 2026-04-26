// Package utils — 백그라운드 알림 중복 제거.
//
// 파일 역할: 동일 TaskID 의 백그라운드 완료 알림을 단일 항목으로 압축한다.
//
//	REPL 메시지 목록을 렌더링하기 전에 적용해 화면 노이즈를 줄인다.
//
// 포함 모듈:
//   - BackgroundNotification: TaskID + Command 를 보유하는 알림 항목
//   - CollapseBackgroundNotifications: TaskID 기준으로 중복 알림 제거
//
// 호출/사용 방식:
//   - internal/repl 의 메시지 렌더링 경로에서 표시 직전에 호출
//
// 연결:
//   - 원본: src/utils/collapseBackgroundBashNotifications.ts
package utils

// BackgroundNotification 은 백그라운드 태스크 완료 알림을 나타낸다.
type BackgroundNotification struct {
	TaskID  string
	Command string
}

// CollapseBackgroundNotifications 는 TaskID 가 동일한 중복 알림을 제거하고
// 처음 등장한 알림만 남긴다. 순서는 유지된다.
func CollapseBackgroundNotifications(notifications []BackgroundNotification) []BackgroundNotification {
	seen := make(map[string]bool, len(notifications))
	result := make([]BackgroundNotification, 0, len(notifications))
	for _, n := range notifications {
		if !seen[n.TaskID] {
			seen[n.TaskID] = true
			result = append(result, n)
		}
	}
	return result
}
