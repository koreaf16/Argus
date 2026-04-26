// Package bash — 명령 완성 스펙 레지스트리.
//
// 파일 역할: fig-style CLI 완성 스펙을 전역 맵에 등록하고 조회하는 기능을 제공한다.
//
//	CommandSpec/OptionSpec/ArgSpec 타입을 정의한다.
//
// 포함 모듈:
//   - CommandSpec: 명령 완성 스펙 (이름, 서브커맨드, 옵션, 인수)
//   - OptionSpec: 옵션 스펙 (이름, 인수)
//   - ArgSpec: 인수 스펙 (명령/모듈/가변/선택적/위험 플래그)
//   - RegisterCommandSpec: 전역 레지스트리에 스펙 등록
//   - LookupCommandSpec: 이름으로 스펙 조회
//
// 호출/사용 방식:
//   - internal/utils/bash/specs/specs.go 가 초기화 시 RegisterCommandSpec 을 호출
//   - internal/utils/bash/prefix.go 가 LookupCommandSpec 을 사용
//
// 연결:
//   - 원본: src/utils/bash/registry.ts
package bash

import "sync"

// ArgSpec 은 명령 인수의 속성을 기술한다.
type ArgSpec struct {
	Name        string
	IsCommand   bool // wrapper 명령인지 (e.g. sudo, timeout)
	IsModule    bool // 모듈 인수인지 (e.g. python -m)
	IsVariadic  bool // 반복 인수인지 (e.g. echo hello world)
	IsOptional  bool // 선택적 인수인지
	IsDangerous bool // 위험한 인수인지
	IsScript    bool // 스크립트 파일인지
}

// OptionSpec 은 명령 옵션(플래그)의 속성을 기술한다.
type OptionSpec struct {
	Name       interface{} // string 또는 []string
	Args       []ArgSpec
	IsRequired bool
}

// CommandSpec 은 CLI 명령의 완성 스펙이다.
type CommandSpec struct {
	Name        interface{} // string 또는 []string
	Description string
	Subcommands []CommandSpec
	Options     []OptionSpec
	Args        interface{} // ArgSpec 또는 []ArgSpec
}

var (
	registryMu     sync.RWMutex
	globalRegistry = map[string]*CommandSpec{}
)

// RegisterCommandSpec 은 name 으로 spec 을 전역 레지스트리에 등록한다.
func RegisterCommandSpec(name string, spec CommandSpec) {
	registryMu.Lock()
	defer registryMu.Unlock()
	copy := spec
	globalRegistry[name] = &copy
}

// LookupCommandSpec 은 name 에 해당하는 CommandSpec 을 반환한다.
// 등록되지 않은 경우 nil, false 를 반환한다.
func LookupCommandSpec(name string) (*CommandSpec, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	spec, ok := globalRegistry[name]
	return spec, ok
}
