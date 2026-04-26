# Shell Provider 설계 (Phase E)

## 1. 개요

InfraCtl 의 shell 실행 경로를 단일 `ShellProvider` 인터페이스로 통합했다.
모든 로컬 명령 실행은 `shell.Resolve() → provider.Prepare()` 를 거친다.

## 2. 인터페이스

```go
// internal/executor/shell/provider.go
type ShellProvider interface {
    Name() string
    Wrap(ctx context.Context, command string) (string, error)   // 하위 호환 shim
    Prepare(ctx context.Context, command string) (PreparedCmd, error)
    Analyze(ctx context.Context, command string) (Analysis, error)
}
```

## 3. 핵심 타입

```go
// Level — 위험도 (iota 비교 금지 — levelPriority() 사용)
type Level int
const (
    LevelLow Level = iota  // 0
    LevelMedium            // 1
    LevelHigh              // 2
    LevelCritical          // 3
    LevelUserApproval      // 4 — FAIL-CLOSED, 승인 필요
)
// 우선순위: Critical(5) > High(4) > UserApproval(3) > Medium(2) > Low(1)

// Analysis — CheckSemantics 반환값
type Analysis struct {
    RiskLevel    Level
    DangerScore  int         // 0..100 (ScoreLow=10, Medium=40, High=70, Critical=100)
    IsReadOnly   bool
    MatchedSpecs []string
    Heredocs     []Heredoc
    PipeCommands []PipeSegment
    Subshells    []string
    Findings     []Finding
    RawCommand   string
    ShellName    string
}

// PreparedCmd — exec.Command 에 바로 사용 가능
type PreparedCmd struct {
    Argv       []string
    Analysis   Analysis
    CleanupFns []func() error  // snapshot 임시 파일 제거
    UsesPTY    bool
}
```

## 4. bash Provider 파이프라인

`Prepare(ctx, command)` 실행 순서:

1. `applyAutoFlags(command)` — specs.Registry AutoFlags 주입 (rm -f, cp -f, mv -f, unzip -q -o)
2. `Parse(ctx, command)` — mvdan.cc/sh/v3 AST 파싱
3. `CheckSemantics(ctx, command, f)` — FAIL-CLOSED 분석
4. `RearrangePipe(command, f)` — 최상위 파이프에 `< /dev/null` 삽입 (stdin 방어)
5. `CreateAndSave(ctx)` — env/pwd/umask/alias/shopt 환경 스냅샷
6. 최종 argv: `["bash", "-c", "source <snap>; set -f; <arranged>"]`
7. `CleanupFns` 에 snap.Cleanup 등록

### FAIL-CLOSED 원칙

- 파싱 실패 → `LevelUserApproval`
- 미식별 CallExpr → `LevelUserApproval`
- 전제어 검사(제어문자/유니코드 공백/zsh 확장) → `LevelUserApproval`

### 위험도 분류

| Level | Score | 대표 예시 |
|---|---|---|
| Critical | 100 | `rm -rf /`, `dd of=/dev/sda`, fork bomb, `curl \| bash` |
| High | 70 | 미식별 명령(FAIL-CLOSED), `sudo`, `chmod 777`, `nc`, `eval` |
| UserApproval | →70 | 분석 불가 패턴, 파싱 실패 |
| Medium | 40 | `rm /tmp/foo`, `mv`, `cp`, `apt install`, `git push` |
| Low | 10 | readonly whitelist 매칭 |

## 5. Readonly Whitelist

`internal/executor/shell/bash/readonly/` 서브패키지:

- `posix.go` — ls/cat/grep/head/tail/find/stat/awk/… (POSIX 기본)
- `git.go` — git log/diff/status/show/… (읽기 전용 git 서브커맨드)
- `gh.go` — gh pr/issue/repo view 계열
- `docker.go` — docker ps/inspect/logs/…
- `rg.go` — ripgrep
- `registry.go` — `IsReadOnly(argv []string) bool` 통합 조회

## 6. PowerShell Provider

- `-EncodedCommand` UTF-16LE base64 인코딩으로 한글/특수문자 안전 전달
- `Analyze` 는 `specs.CheckDangerous` (substring 패턴) 기반 — AST 없음
- 미식별 명령 → `LevelUserApproval` (FAIL-CLOSED)

## 7. hook_metadata.go 통합

`ComputeMetadata("bash", args)` 는 항상 `shell.Lookup("bash")→Analyze()` 를 사용한다.
(인프라 명령은 POSIX 스타일이므로 OS 와 무관하게 bash 분석이 정확하다.)

`Analysis.DangerScore` → `HookMetadata.DangerScore` 변환:
- ≥ 70 → "high"
- 40–69 → "medium"
- < 40 → "low"

## 8. 검증

- `go test ./internal/executor/shell/bash/...` — AST 단위 테스트
- `go test ./internal/agent/... -run Equivalence` — 55 케이스 동등성 (deny/allow/readonly)
- `go test ./...` — 30 패키지 전부 통과
