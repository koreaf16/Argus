// Package readonly
// File: types.go
// Description: 읽기 전용 명령어 검증에 쓰이는 공유 타입 정의
// Responsibility: FlagArgType, CommandConfig 타입 및 헬퍼

// Ported from: claude_cli/src/utils/shell/readOnlyCommandValidation.ts (types section)

package readonly

// FlagArgType 은 플래그가 받는 인자의 종류를 나타낸다.
// Ported from: claude_cli/src/utils/shell/readOnlyCommandValidation.ts:FlagArgType
type FlagArgType int

const (
	// FlagArgNone 은 인자를 받지 않는 플래그 (예: --color, -n)
	FlagArgNone FlagArgType = iota
	// FlagArgNumber 는 정수 인자 (예: --context=3)
	FlagArgNumber
	// FlagArgString 은 임의 문자열 인자 (예: --relative=path)
	FlagArgString
	// FlagArgChar 는 단일 문자 인자 (구분자 등)
	FlagArgChar
	// FlagArgLiteralBraces 는 리터럴 "{}" 만 허용
	FlagArgLiteralBraces
	// FlagArgLiteralEOF 는 리터럴 "EOF" 만 허용
	FlagArgLiteralEOF
)

// CommandConfig 는 하나의 명령어(또는 서브커맨드)에 대한 안전 플래그 목록이다.
// Ported from: claude_cli/src/utils/shell/readOnlyCommandValidation.ts:ExternalCommandConfig
type CommandConfig struct {
	// SafeFlags 는 이 명령어에서 안전하게 허용되는 플래그 → 인자 타입 매핑이다.
	SafeFlags map[string]FlagArgType
	// IsDangerous 는 추가적인 위험성 판단 콜백이다. nil 이면 사용 안 함.
	// rawCommand 와 플래그 제거 후 남은 positional args 를 받는다.
	IsDangerous func(rawCommand string, args []string) bool
	// RespectsDoubleDash 는 '--' 이후 인자 검사를 중단할지 여부다.
	// false 이면 '--' 이후에도 플래그를 계속 검사한다. 기본값 true.
	RespectsDoubleDash *bool
}

func boolPtr(b bool) *bool { return &b }

// respectsDD 는 nil 이면 true (기본값), 아니면 포인터 값을 반환한다.
func (c CommandConfig) respectsDD() bool {
	if c.RespectsDoubleDash == nil {
		return true
	}
	return *c.RespectsDoubleDash
}
