// Package permissions — 위험한 셸 명령 패턴 목록.
//
// 파일 역할: auto 모드에서 분류기 검토를 트리거해야 하는 인터프리터·도구 이름 목록.
//
//	이 목록에 포함된 접두사로 시작하는 Bash 허용 규칙은 auto 모드 진입 시 제거된다.
//
// 포함 모듈:
//   - CrossPlatformCodeExec: Unix/Windows 공통 코드 실행 진입점.
//   - DangerousBashPatterns: Bash 권한 규칙에서 위험한 접두사 전체 목록.
//
// 호출/사용 방식:
//   - internal/utils/permissions/permission_setup.go 의 isDangerousBashPermission() 에서 참조.
//
// 연결:
//   - 원본: src/utils/permissions/dangerousPatterns.ts
package permissions

// CrossPlatformCodeExec 는 Unix/Windows 공통 코드 실행 진입점 목록이다.
var CrossPlatformCodeExec = []string{
	// 인터프리터
	"python", "python3", "python2",
	"node", "deno", "tsx",
	"ruby", "perl", "php", "lua",
	// 패키지 실행기
	"npx", "bunx",
	"npm run", "yarn run", "pnpm run", "bun run",
	// 셸 (Git Bash / WSL on Windows, 네이티브 on Unix)
	"bash", "sh",
	// 원격 임의 명령 래퍼
	"ssh",
}

// DangerousBashPatterns 는 Bash 허용 규칙에서 위험한 명령 접두사 목록이다.
// 이 패턴으로 시작하는 규칙은 auto 모드에서 분류기 검토를 받아야 한다.
var DangerousBashPatterns = func() []string {
	patterns := make([]string, 0, len(CrossPlatformCodeExec)+10)
	patterns = append(patterns, CrossPlatformCodeExec...)
	patterns = append(patterns,
		"zsh", "fish",
		"eval", "exec", "env", "xargs",
		"sudo",
	)
	return patterns
}()
