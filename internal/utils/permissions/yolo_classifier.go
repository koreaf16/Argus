// Package permissions — YOLO(bypassPermissions) 모드 분류기 스텁.
//
// 파일 역할: bypassPermissions 모드에서 작업의 안전성을 AI 분류기로 판단한다.
//
//	Phase 1 에서는 스텁으로 항상 비차단을 반환한다.
//	Phase 2 에서 LLM 기반 분류기로 교체 예정.
//
// 포함 모듈:
//   - ClassifyYoloAction(): 작업 안전성 분류.
//
// 호출/사용 방식:
//   - bypassPermissions 모드에서 위험 작업 감지 시 참조.
//
// 연결:
//   - 원본: src/utils/permissions/yoloClassifier.ts
package permissions

import (
	"context"

	"github.com/koreaf16/argus/internal/types"
)

// YoloClassifierResult 는 YOLO 분류기 결과다.
type YoloClassifierResult struct {
	ShouldBlock bool
	Reason      string
	Unavailable bool
	Model       string
}

// ClassifyYoloAction 은 bypassPermissions 모드에서 작업의 안전성을 분류한다.
// Phase 1 스텁: 항상 비차단을 반환한다.
func ClassifyYoloAction(
	ctx context.Context,
	toolName string,
	input []byte,
	description string,
) YoloClassifierResult {
	return YoloClassifierResult{
		ShouldBlock: false,
		Reason:      "YOLO 분류기가 아직 구현되지 않았습니다 (Phase 2 예정).",
		Unavailable: true,
		Model:       string(types.ClassifierBehaviorAllow),
	}
}
