// Package lifecycle
// File: matcher.go
// Description: 훅 조건 매칭 로직
// Responsibility: Hook의 Condition 필드를 파싱하여 HookContext와 매칭 여부를 판단

package lifecycle

import (
	"strings"

	"github.com/yourorg/infractl/internal/store"
)

// Matches는 훅의 조건이 주어진 컨텍스트와 매칭되는지 판단한다.
// 조건이 비어 있으면 항상 매칭된다.
// 조건 형식: "key=value,key2=value2" (콤마 구분 AND 조건)
// 지원 키: tool, server, service
func Matches(h store.Hook, hctx HookContext) bool {
	if h.Condition == "" {
		return true
	}
	for _, pair := range strings.Split(h.Condition, ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.TrimSpace(kv[1])
		switch key {
		case "tool":
			if hctx.ToolName != val {
				return false
			}
		case "server":
			if hctx.Server != val {
				return false
			}
		case "service":
			if hctx.ServiceType != val {
				return false
			}
		}
	}
	return true
}
