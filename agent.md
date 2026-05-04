# Argus Agent 작업 지침

## 커뮤니케이션 규칙
- 모든 보고와 사전 계획은 반드시 한국어로 작성합니다.
- 모든 프롬프트는 반드시 한국어로 작성합니다.
- 사용자 응답은 반드시 존댓말로 작성합니다.
- **UI 관련 제안이나 변경 사항 보고 시, 반드시 '랜더링 예상도'를 포함하여 시각적으로 어떻게 보이는지 보고해야 합니다.**

## 프로젝트 원칙
- 별도 지시가 없으면 모든 작업 대상은 `Argus.exe` 프로젝트입니다.
- 구현 중 막히는 항목은 `C:\Dev\Argus\claude_cli_참고용` 구조/동작을 우선 기준으로 해결합니다.
- 연속 작업과 테스트는 `session_id` + `--resume`(`-r`)를 사용해 맥락을 유지합니다.
- 자동화 테스트성 출력이 필요할 때는 `--aidebug`를 사용합니다.
- **`--aidebug`로 확인하라는 지시를 받으면, 코드를 먼저 열어보지 말고 반드시 `./Argus.exe --aidebug -p "<메시지>"` 를 실행해 실제 출력값을 먼저 확인한 뒤 판단합니다.**

## 구현 원칙
- 원본 TS 코드를 그대로 복붙하지 말고, Go 코드로 재구성합니다.
- **UI 구현 시 `AltScreen`(fullscreen TUI) 모드는 사용 금지이며, 터미널 스크롤백이 유지되면서 하단 상태바가 고정되고 답변이 흐르듯이 출력되는 Dynamic Inline Rendering 방식을 사용해야 합니다.**
- 파일은 500라인 이하를 유지하고, 커지면 책임 단위로 분리합니다.
- 기능 추가/변경 시 관련 `docs/` 문서를 같은 변경에서 함께 갱신합니다.
- 파괴적 명령(`reset --hard`, 무분별한 삭제/복원)은 사용자 명시 요청 없이는 금지합니다.

## 도구(Tool) 명명 규칙 — 반드시 준수

### Name()은 항상 snake_case
```go
// ✅ 올바름
func (t *MyTool) Name() string { return "my_tool" }

// ❌ 금지 — PascalCase, camelCase 모두 불가
func (t *MyTool) Name() string { return "MyTool" }
func (t *MyTool) Name() string { return "myTool" }
```

**이유**: `CanonicalName()`, `safeAutoModeTools`, `transcript.go`의 `isTaskMutationTool` 등
코드베이스 전반에서 도구명을 문자열로 직접 비교합니다.
대소문자 불일치 시 조용히 실패하며 디버깅이 매우 어렵습니다.
실제로 `"TaskCreate"`, `"EnterWorktreeTool"` 등 PascalCase 이름이 이 문제를 일으켰습니다.

### 새 도구 추가 체크리스트
새 도구를 만들면 반드시 다음 세 곳을 모두 처리합니다:

1. **`cmd/argus/main.go`** — import + toolRegistry 등록
   ```go
   import "github.com/koreaf16/argus/internal/tools/mytool"
   // bootstrap() 내 toolRegistry 루프에 추가:
   mytool.New(),
   ```

2. **`internal/utils/permissions/classifier_decision.go`** — `safeAutoModeTools` 맵에 추가
   - 읽기 전용 또는 안전한 도구라면 `"my_tool": true` 추가
   - 키는 `Name()` 반환값과 **정확히 일치**해야 함

3. **`internal/tools/canonical.go`** — 레거시/별칭이 필요한 경우에만 추가
   - 이름이 바뀌거나 alias가 필요한 경우 등록
   - 자기참조(`"mytool" → "mytool"`) 항목은 추가하지 않음

### 도구명이 쓰이는 위치 (수정 시 함께 확인)
| 위치 | 체크 포인트 |
|------|------------|
| `internal/tools/canonical.go` | alias 맵 키/값 |
| `internal/tools/evidence.go` | `ToolEvidenceCategories` switch case |
| `internal/tui/transcript.go` | `isTaskMutationTool`, `isCollapsibleTool`, `isBashTool` |
| `internal/tui/toolui/inline.go` | `NormalizeToolName` switch case |
| `internal/utils/permissions/classifier_decision.go` | `safeAutoModeTools` 맵 |
| `internal/constants/tools.go` | 상수로 관리하는 도구명 |

## 참고 문서
- 아키텍처: `docs/architecture.md`
- 단계 계획: `docs/phase1.md`
- 변경 이력: `docs/CHANGELOG.md`
