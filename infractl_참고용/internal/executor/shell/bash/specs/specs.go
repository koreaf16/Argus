// Package specs
// File: specs.go
// Description: bash 명령어 스펙 레지스트리 — CommandSpec 정의 및 알려진 명령어 목록
// Responsibility: 자동 플래그 주입 및 명령어별 특수 처리 정보 제공

// Ported from: claude_cli/src/utils/bash/specs/*.ts (timeout, sleep, alias, nohup, time, pyright, srun)

package specs

// FlagSpec 은 명령어가 자동으로 주입받아야 하는 플래그를 나타낸다.
type FlagSpec struct {
	// Flag 는 주입할 플래그 문자열 (예: "-q")
	Flag string
	// IfMissing 이 true 이면 해당 플래그가 아직 없을 때만 주입한다
	IfMissing bool
}

// CommandSpec 은 특정 명령어에 대한 자동 처리 정보를 담는다.
// Ported from: claude_cli/src/utils/bash/specs/*.ts:CommandSpec
type CommandSpec struct {
	// Name 은 명령어 이름 (예: "unzip", "cp")
	Name string
	// AutoFlags 는 비대화형 실행을 위해 자동 주입할 플래그 목록이다
	AutoFlags []FlagSpec
	// Passthrough 가 true 이면 명령어를 그대로 통과시킨다 (분석만 수행)
	Passthrough bool
}

// Registry 는 알려진 CommandSpec 목록을 이름으로 조회한다.
var Registry = map[string]CommandSpec{
	// 비대화형 모드 강제 — Phase A 의 InjectNonInteractiveFlags 에서 이관
	// Ported from: claude_cli/src/utils/bash/specs/timeout.ts (패턴 참고)
	"unzip": {Name: "unzip", AutoFlags: []FlagSpec{{Flag: "-o", IfMissing: true}}},
	"cp":    {Name: "cp", AutoFlags: []FlagSpec{{Flag: "-f", IfMissing: true}}},
	"mv":    {Name: "mv", AutoFlags: []FlagSpec{{Flag: "-f", IfMissing: true}}},
	"rm":    {Name: "rm", AutoFlags: []FlagSpec{{Flag: "-f", IfMissing: true}}},

	// 시간 래퍼 — Passthrough (분석만)
	// Ported from: claude_cli/src/utils/bash/specs/timeout.ts
	"timeout": {Name: "timeout", Passthrough: true},
	// Ported from: claude_cli/src/utils/bash/specs/time.ts
	"time": {Name: "time", Passthrough: true},
	// Ported from: claude_cli/src/utils/bash/specs/nohup.ts
	"nohup": {Name: "nohup", Passthrough: true},
	// Ported from: claude_cli/src/utils/bash/specs/sleep.ts
	"sleep": {Name: "sleep", Passthrough: false},
	// Ported from: claude_cli/src/utils/bash/specs/alias.ts
	"alias": {Name: "alias", Passthrough: false},
	// Ported from: claude_cli/src/utils/bash/specs/pyright.ts
	"pyright": {Name: "pyright", Passthrough: false},
	// Ported from: claude_cli/src/utils/bash/specs/srun.ts
	"srun": {Name: "srun", Passthrough: false},
}

// Lookup 은 명령어 이름으로 CommandSpec 을 반환한다.
func Lookup(name string) (CommandSpec, bool) {
	s, ok := Registry[name]
	return s, ok
}

// AutoInjectFlags 는 명령어 argv 에 CommandSpec 의 AutoFlags 를 주입한다.
// IfMissing 이 true 인 플래그는 이미 존재하면 건너뛴다.
func AutoInjectFlags(argv []string, spec CommandSpec) []string {
	if len(argv) == 0 || len(spec.AutoFlags) == 0 {
		return argv
	}
	result := make([]string, len(argv))
	copy(result, argv)

	for _, fs := range spec.AutoFlags {
		if !fs.IfMissing {
			result = append(result, fs.Flag)
			continue
		}
		already := false
		for _, a := range result[1:] { // 명령어 이름 제외
			if a == fs.Flag {
				already = true
				break
			}
		}
		if !already {
			// 명령어 이름 바로 뒤에 삽입
			result = append(result[:1], append([]string{fs.Flag}, result[1:]...)...)
		}
	}
	return result
}
