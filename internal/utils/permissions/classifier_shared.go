// Package permissions — 분류기 공통 인프라.
//
// 파일 역할: Bash 명령 분류기와 YOLO 분류기에서 공통으로 사용하는
//
//	도구 호출 블록 추출·파싱 유틸리티를 제공한다.
//	Phase 1 에서는 LLM 분류기가 스텁이므로 이 파일도 기본 구조만 제공한다.
//
// 포함 모듈:
//   - ClassifierToolUseBlock: 분류기 도구 호출 블록 타입.
//   - ExtractToolUseBlock(): 메시지 콘텐츠에서 도구 호출 블록 추출.
//
// 호출/사용 방식:
//   - internal/utils/permissions/bash_classifier.go 에서 참조.
//   - internal/utils/permissions/yolo_classifier.go 에서 참조.
//
// 연결:
//   - 원본: src/utils/permissions/classifierShared.ts
package permissions

import "encoding/json"

// ClassifierToolUseBlock 은 LLM 분류기가 반환하는 도구 호출 블록이다.
type ClassifierToolUseBlock struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ExtractToolUseBlock 은 콘텐츠 블록 목록에서 지정 이름의 tool_use 블록을 추출한다.
// 해당 블록이 없으면 nil 을 반환한다.
func ExtractToolUseBlock(content []ClassifierToolUseBlock, toolName string) *ClassifierToolUseBlock {
	for i := range content {
		if content[i].Type == "tool_use" && content[i].Name == toolName {
			return &content[i]
		}
	}
	return nil
}
