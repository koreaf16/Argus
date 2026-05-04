# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 커뮤니케이션 규칙
- 모든 보고와 사전 계획은 반드시 한국어로 작성합니다.
- 모든 프롬프트는 반드시 한국어로 작성합니다.
- 사용자 응답은 반드시 존댓말로 작성합니다.
- **UI 관련 제안이나 변경 사항 보고 시, 반드시 '랜더링 예상도'를 포함하여 시각적으로 어떻게 보이는지 보고해야 합니다.**

## 프로젝트 기본 원칙
- 별도 지시가 없으면 모든 작업 대상은 `Argus.exe` 프로젝트입니다.
- UI/동작 구현은 `C:\Dev\Argus\claude_cli_참고용` 구조를 우선 기준으로 맞춥니다.
- **UI 구현 시 `AltScreen`(fullscreen TUI) 모드를 사용하지 않습니다. 터미널 스크롤백이 유지되면서 하단 상태바가 고정되고 답변이 흐르듯이 출력되는 Dynamic Inline Rendering 방식을 사용해야 합니다. (gemini_cli / claude_cli 방식)**
- 연속 실행/검증 시 `session_id`와 `--resume`(`-r`)를 사용해 세션 맥락을 유지합니다.
- 자동화 테스트성 출력이 필요할 때는 `--aidebug`를 사용합니다.
- **`--aidebug`로 확인하라는 지시를 받으면, 코드를 먼저 열어보지 말고 반드시 `./Argus.exe --aidebug -p "<메시지>"` 를 실행해 실제 출력값을 먼저 확인한 뒤 판단합니다.**
- 파일은 500라인 이하를 유지하고, 커지면 책임 단위로 분리합니다.

## 도구(Tool) 명명 규칙
- **`Name()` 반환값은 반드시 `snake_case`** (소문자 + 언더스코어). `"my_tool"` ✅ / `"MyTool"` ❌
- 대소문자 불일치 시 `CanonicalName()`, `safeAutoModeTools`, `transcript.go` 등에서 조용히 실패함
- 새 도구 추가 시 반드시: ① `cmd/argus/main.go` 등록 ② `classifier_decision.go` `safeAutoModeTools` 추가 ③ `canonical.go` alias (필요 시)
- 상세 체크리스트는 `agent.md` → "도구(Tool) 명명 규칙" 섹션 참조

## 상세 규칙 참조
- 구현/아키텍처/문서 동기화 규칙은 `agent.md`를 기준으로 따릅니다.

---

## 빌드 및 테스트 명령어

```bash
# 빌드
go build -o Argus.exe ./cmd/argus

# 전체 테스트
go test ./...

# 특정 패키지 테스트
go test -v ./internal/query/...
go test -v ./internal/tui/...
go test -v ./internal/context/...

# 특정 테스트 하나만 실행
go test -run TestFunctionName ./internal/query/...

# 포맷 / 린트
go fmt ./...
go vet ./...

# 의존성 정리
go mod tidy
```

---

## 아키텍처 개요

**모듈**: `github.com/koreaf16/argus` (Go 1.23.0)

### 데이터 흐름

```
User Input
  └─ internal/tui/tui.go          (Bubbletea 이벤트 루프)
       └─ engine.SubmitMessage()
            └─ internal/query/engine.go   (대화 엔진)
                 ├─ context.Graph.AppendUser()       ← episodic graph 관리
                 ├─ LLM.Stream()                     ← Anthropic/Gemini/OpenAI
                 ├─ Tool.Call() → <-chan ToolEvent   ← tool 실행 (비동기 채널)
                 ├─ Distiller.Distill()              ← 결과 압축 FULL/PARTIAL/SUMMARY
                 └─ emit UIEvent (channel)
                      └─ tui.go가 수신 → tea.Println() 스크롤백 출력
```

### 핵심 패키지

| 패키지 | 역할 |
|--------|------|
| `cmd/argus/` | CLI 진입점. `--print`, `--resume`, `--aidebug` 등 플래그 처리 |
| `internal/query/` | LLM 스트림 처리 + 도구 실행 루프. `engine.go`가 중심 |
| `internal/context/` | Episodic Context Graph. `graph.go`가 대화 이력 노드 관리, 토큰 예산 초과 시 비보호 노드 압축 |
| `internal/tui/` | Bubbletea 기반 Dynamic Inline Rendering. `model.go`(Update/View), `render.go`(렌더링), `transcript.go`(이벤트→엔트리 변환) |
| `internal/presentation/` | `query.UIEvent` → `presentation.Event` 변환 계층. TUI와 query 간 결합도 분리 |
| `internal/services/llm/` | `LLM` 인터페이스 구현 (Anthropic / Gemini / OpenAI-compat). `toolcalls.go`가 tool_use ↔ function_call 형식 변환 |
| `internal/tools/` | 31개 도구 + Registry. 각 도구는 `Tool` 인터페이스를 구현하고 `<-chan ToolEvent`로 비동기 결과 반환 |
| `internal/services/mcp/` | MCP 서버 프로세스 관리 + Argus Tool로 래핑 |
| `internal/services/workspace/` | SSH 워크스페이스 연결, 비밀번호 캐싱 |
| `internal/memdir/` | `~/.argus/` JSON 파일 영속화 (세션, 설정, 아티팩트) |
| `internal/state/` | 런타임 앱 상태 (활성 모델, 워크스페이스, 권한 모드) |

### Tool Renderer 플러그인 시스템

도구별 커스텀 UI는 `internal/tui/toolui/renderer.go`의 `ToolRenderer` 인터페이스로 등록한다. 각 도구 패키지의 `ui.go`에서 `init()`으로 자동 등록한다.

```go
// 예: internal/tools/webfetch/ui.go
func init() { toolui.Register("webfetch", &WebFetchRenderer{}) }

type ToolRenderer interface {
    RenderToolUse(args map[string]any, theme ThemeContext) string     // tool_use 표시
    RenderToolResult(resultText string, durationMs int64, theme ThemeContext) string  // tool_result 표시
}
```

`render.go`에서 `tool_use` 엔트리의 Body(= JSON args)를 언마샬해 `RenderToolUse`로 전달한다. 렌더러가 없으면 기본 title+body 표시.

### TUI 렌더링 구조

```
View() 반환값 (anchored, 화면 하단 고정)
  ├─ 권한 승인 모달 (선택적)
  ├─ 스트리밍 중인 assistant/thinking 영역
  ├─ 입력박스 (top-border-only 스타일)
  └─ footer (상태줄 + 단축키 힌트)

tea.Println() → 터미널 스크롤백 (완료된 메시지 고정)
```

`transcript.go`가 `presentation.Event`를 `transcriptEntry`로 변환하고, `render.go`가 각 엔트리를 ANSI 문자열로 렌더링한다.
읽기/검색 도구(grep, glob, webfetch 등)는 `tool_group`으로 묶어 하나의 접이식 행으로 표시한다.

### 컨텍스트 압축 규칙 (Distiller)

- **FULL** (≤24k chars): 결과를 인라인 그대로 graph에 저장
- **PARTIAL** (24k–32k): head+tail만 인라인, 전체는 artifact 파일로 저장
- **SUMMARY** (>32k): 요약만 인라인, 전체는 artifact 파일로 저장
- Artifact 경로: `~/.argus/session-artifacts/<sessionID>/<seq>-<tool>-<id>.txt`

보호된 노드(최근 2턴, 마지막 tool_use/result 쌍 등)는 압축 대상에서 제외.

### 설정 파일 위치

| 파일 | 역할 |
|------|------|
| `~/.argus/settings.json` | 훅, 권한 정책, 프리셋 |
| `~/.argus/models.json` | 모델 카탈로그 (alias → provider/model_id) |
| `~/.argus/sessions/<id>.json` | 세션 스냅샷 (v2: graph + artifact refs) |
| `.Argus/settings.json` | 프로젝트 로컬 설정 (home 설정과 병합) |
