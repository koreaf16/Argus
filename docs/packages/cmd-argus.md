# cmd/argus — 진입점

## 파일 구조

| 파일 | 역할 |
|------|------|
| `main.go` | 메인 진입점, 플래그 파싱, 부트스트랩 |
| `help.go` | 도움말 출력 |
| `init.go` | `--init` 모드 처리 |
| `init_menu.go` | 대화형 초기화 메뉴 |
| `init_prompt.go` | 초기화 프롬프트 |
| `run_print.go` | `--print` 모드 처리 |
| `version.go` | 버전 정보 |

## 플래그

| 플래그 | 타입 | 설명 |
|--------|------|------|
| `--help` / `-h` | bool | 도움말 |
| `--version` / `-v` | bool | 버전 |
| `--init` | bool | 설정 파일 생성 |
| `--model <alias>` | string | 세션 활성 모델 |
| `--print` / `-p <prompt>` | string | 싱글 프롬프트 모드 |
| `--resume` / `-r <id>` | string | 세션 복원 |
| `--aidebug` | bool | NDJSON 트레이스 출력 |
| `--auto-approve` | bool | Tool 권한 자동 승인 |

## 부트스트랩 흐름

[`main.go:bootstrap()`](cmd/argus/main.go) 는 다음 순서로 초기화합니다:

1. **Memdir**: `~/.argus` 디렉토리 구조 생성
2. **LLM Registry**: `~/.argus/models.json` 로드
3. **AppState**: 앱 상태 생성
4. **Session**: UUIDv4 생성 또는 `--resume` 복원
5. **Tool Registry**: 내장 23개 도구 + MCP 브리지 등록
6. **MCP Manager**: `~/.argus/mcp.json` 로드
7. **LSP Manager**: Language Server 초기화
8. **Skills Registry**: `~/.argus/skills/` 로드
9. **Hook Dispatcher**: `~/.argus/settings.json` 훅 설정 로드
10. **Query Engine**: 모든 종속성 주입
11. **ShellJobs Manager**: 백그라운드 작업 관리

## 모드 라우팅

| 조건 | 모드 | 설명 |
|------|------|------|
| 기본 | TUI | BubbleTea 기반 Full TUI |
| `--aidebug` | AIDebug | NDJSON 트레이스 + REPL |
| `--print <prompt>` | Print | 싱글 프롬프트 → 결과 출력 |
