// Package executor
// File: idle.go
// Description: 명령 출력 유휴 상태 감지 타이머
// Responsibility: 스트리밍 실행 중 일정 시간 출력이 없으면 Triggered 채널에 신호를 보내 알림.
//                 한 번 트리거 후에도 계속 감시하여 반복 프롬프트(예: 비밀번호 재입력)를 감지한다.

package executor

import (
	"context"
	"time"
)

const defaultIdleThreshold = 3 * time.Second

// IdleWatcher는 마지막 Ping 이후 threshold 시간 동안 추가 Ping이 없으면
// Triggered() 채널에 신호를 보내 호출자에게 유휴 상태를 알린다.
// 트리거 후에도 계속 동작하여 다음 유휴 상태를 반복 감지한다.
type IdleWatcher struct {
	threshold time.Duration
	resetCh   chan struct{}
	triggerCh chan struct{}
	stopCh    chan struct{}
}

// NewIdleWatcher는 지정된 threshold로 IdleWatcher를 생성한다.
// threshold가 0이면 defaultIdleThreshold(10초)를 사용한다.
func NewIdleWatcher(threshold time.Duration) *IdleWatcher {
	if threshold == 0 {
		threshold = defaultIdleThreshold
	}
	return &IdleWatcher{
		threshold: threshold,
		resetCh:   make(chan struct{}, 16),
		triggerCh: make(chan struct{}, 4), // buffered: 반복 트리거 수신 가능
		stopCh:    make(chan struct{}),
	}
}

// Start는 백그라운드 감시 고루틴을 시작한다. 반드시 한 번만 호출해야 한다.
// ctx가 취소되거나 Stop이 호출되면 고루틴이 종료된다.
func (w *IdleWatcher) Start(ctx context.Context) {
	go w.run(ctx)
}

// Stop은 감시 고루틴을 종료한다.
func (w *IdleWatcher) Stop() {
	select {
	case <-w.stopCh:
		// 이미 종료됨
	default:
		close(w.stopCh)
	}
}

// Ping은 새 출력 라인이 수신될 때마다 호출한다. 유휴 타이머를 리셋한다.
func (w *IdleWatcher) Ping() {
	select {
	case w.resetCh <- struct{}{}:
	default:
		// 버퍼가 가득 찬 경우 드롭 — 고루틴이 곧 소비하므로 안전
	}
}

// Triggered는 유휴 threshold 초과 시 신호가 오는 채널을 반환한다.
// 트리거마다 값이 전송되므로 for-select 루프로 반복 수신 가능하다.
func (w *IdleWatcher) Triggered() <-chan struct{} {
	return w.triggerCh
}

func (w *IdleWatcher) run(ctx context.Context) {
	timer := time.NewTimer(w.threshold)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-w.resetCh:
			drainResets(w.resetCh)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(w.threshold)
		case <-timer.C:
			// 유휴 감지 — 채널에 신호 전송 후 타이머 재시작
			select {
			case w.triggerCh <- struct{}{}:
			default:
				// 수신자가 아직 처리 중이면 신호를 드롭하고 계속 진행
			}
			timer.Reset(w.threshold)
		}
	}
}

// drainResets는 resetCh에 쌓인 신호를 모두 비운다.
func drainResets(ch <-chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}
