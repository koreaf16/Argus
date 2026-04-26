// Package types — 플러그인 타입 정의.
//
// 파일 역할: 사용자 정의 플러그인(커맨드, 훅, MCP 서버 등)의 기본 타입을 정의한다.
//
//	Phase 1 에서는 BuiltinPluginDefinition 과 LoadedPlugin 만 포함한다.
//	Phase 2+에서 PluginManifest, PluginError 등으로 확장된다.
//
// 포함 모듈:
//   - BuiltinPluginDefinition: 내장 플러그인 정의.
//   - LoadedPlugin: 로드된 플러그인 상태.
//   - PluginLoadResult: 플러그인 로드 결과 요약.
//
// 호출/사용 방식:
//   - internal/repl/commands/ 및 부팅 시 플러그인 로드 시 참조.
//
// 연결:
//   - 원본: src/types/plugin.ts (Phase 1 핵심만 포함)
package types

// BuiltinPluginDefinition 은 CLI 에 내장된 플러그인 정의다.
type BuiltinPluginDefinition struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Version        string `json:"version,omitempty"`
	DefaultEnabled bool   `json:"defaultEnabled,omitempty"`
	// IsAvailable 이 nil 이면 항상 사용 가능.
	IsAvailable func() bool `json:"-"`
}

// LoadedPlugin 은 런타임에 로드된 플러그인 상태다.
type LoadedPlugin struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Source    string `json:"source"`
	Enabled   bool   `json:"enabled,omitempty"`
	IsBuiltin bool   `json:"isBuiltin,omitempty"`
}

// PluginLoadResult 는 플러그인 로드 결과다.
type PluginLoadResult struct {
	Enabled  []LoadedPlugin `json:"enabled"`
	Disabled []LoadedPlugin `json:"disabled"`
	Errors   []string       `json:"errors,omitempty"`
}
