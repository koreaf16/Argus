# Argus query/tools/TODO/web/parallel audit

Last verified: 2026-04-28
Workspace: `C:\Dev\Argus`
Scope: `HEAD` plus current dirty working tree, including untracked workflow/task-plan files.
Reference compared: local `claude_cli_참고용` source.

## 1. Executive summary

현재 작업트리의 핵심 리스크는 "도구 이름, workflow phase 정책, read-only/parallel 판정, TODO 상태 저장소"가 서로 다른 기준으로 움직인다는 점이다. 이 상태에서는 모델 프롬프트가 허용한 도구를 엔진이 거부하거나, 반대로 phase guide가 금지한 도구가 엔진에서 통과될 수 있다. 특히 research phase의 `webfetch` 차단 가능성은 current/latest 질의에서 web evidence gate와 직접 충돌한다.

Claude reference와 비교하면 Argus는 parallel batch 개념은 도입되어 있지만, schema validation, panic recovery, concurrency cap, streaming-time execution, sibling abort, context modifier 후처리 같은 보호 장치가 아직 빠져 있다. 또한 일부 상태 변경 도구가 `IsReadOnly() == true`로 등록되어 있어서 parallel-safe로 취급될 수 있다.

검증 결과 기준으로 현재 재현되는 실패는 다음과 같다.

```text
go test ./internal/query ./internal/state ./internal/services/tools ./internal/tools/websearch ./internal/tools/webfetch

FAIL github.com/koreaf16/argus/internal/query
- TestTriggerWorkflowHeuristic
- TestDefaultSystemPrompt_IncludesGreetingGuidance

FAIL github.com/koreaf16/argus/internal/state
- TestSetWorkflowCardClonesConstraints

ok github.com/koreaf16/argus/internal/services/tools
ok github.com/koreaf16/argus/internal/tools/websearch
ok github.com/koreaf16/argus/internal/tools/webfetch
```

## 2. P0 issues

### P0-1. Research phase에서 실제 `webfetch`가 차단될 수 있음

Evidence:
- 실제 WebFetch tool name은 `internal/tools/webfetch/webfetch.go:21`의 `webfetch`.
- `internal/query/workflow.go:48` research allow-list는 `web_fetch`를 허용하고 `webfetch`를 빠뜨림.
- `internal/tools/taskplaninit/taskplaninit.go:254` phase guide는 `webfetch` 사용을 지시하고, `internal/tools/taskplaninit/taskplaninit.go:276`의 canonical list도 `webfetch`를 포함한다.
- `internal/query/web_evidence.go:280` web evidence 성공 감지도 `webfetch` 이름을 본다.

Impact:
- 최신/현재 정보 질의에서 시스템 프롬프트와 web evidence gate는 `web_search` 뒤 `webfetch`를 요구하지만, workflow research gate가 실제 `webfetch` 호출을 막을 수 있다.
- 모델은 follow-up을 받아도 같은 도구가 계속 차단되어 반복 실패하거나 snippet-only 답변을 강제로 거절당할 수 있다.

Fix:
- 도구 이름 canonicalization helper를 만든다. 최소 alias: `web_fetch -> webfetch`, `websearch -> web_search`, case/trim normalization.
- `workflowPhaseBlocks`는 hard-coded map을 제거하고 `taskplaninit.PhaseAllowedTools`를 단일 소스로 사용한다.
- UI renderer와 collapsible classification도 같은 canonical name을 사용한다.

Acceptance:
- research phase에서 `webfetch`가 통과하는 단위 테스트 추가.
- `web_fetch` alias가 들어와도 `webfetch` renderer/evidence/UI로 정상 라우팅되는 테스트 추가.

### P0-2. Workflow phase guide와 엔진 enforcement가 서로 다름

Evidence:
- `internal/query/workflow.go:46-50`에 별도 allow-list가 있음.
- `internal/tools/taskplaninit/taskplaninit.go:271-282`에 또 다른 phase allow-list가 있음.
- research: `workflow.go`는 `web_fetch`, `google_web_search` 중심이고 MCP/file/glob/grep 일부가 빠짐. `taskplaninit`은 `webfetch`, MCP, file tools를 포함.
- plan: `workflow.go`는 `bash`, `powershell`, `server_inspect`를 허용하지만 `taskplaninit`은 `enter_plan_mode`, `exit_plan_mode`를 지시한다.
- verify/done: `workflow.go:41`은 `execute`, `verify`, `done`을 무제한 통과시킨다. `taskplaninit.go:281-282`는 verify 도구를 제한한다.

Impact:
- 프롬프트는 `enter_plan_mode/exit_plan_mode`를 쓰라 하지만 엔진은 plan phase에서 다른 shell 도구까지 허용한다.
- verify phase가 사실상 무제한이 되어 phase-based safety의 의미가 줄어든다.

Fix:
- `PhaseAllowedTools`를 state/tool package의 단일 정책으로 승격한다.
- `workflowPhaseBlocks`는 `nil`이면 unrestricted, non-nil이면 canonical name 기준 allow만 수행한다.
- phase guide text, engine enforcement, tests가 같은 list를 공유하게 만든다.

Acceptance:
- discover/research/interview/plan/verify phase별 allowed/blocked matrix test.
- guide string에 나온 도구가 enforcement에서 모두 허용되는 coverage test.

### P0-3. Plan phase의 모든 `TodoWrite`가 approval gate에 막힘

Evidence:
- `internal/query/workflow.go:33-36`은 current phase가 `plan`이고 tool name이 `TodoWrite`면 input을 보지 않고 `user_approved`를 요구한다.
- `internal/tools/todowrite/todowrite.go:114-115`는 `req.Phase`가 있을 때 workflow phase를 갱신한다.
- `internal/tools/askuser/ask_user_tool.go:159`는 답변 텍스트에 승인 표현이 있으면 전역 metadata `user_approved`를 true로 둔다.

Impact:
- plan phase에서 단순 checklist 정리, plan TODO 갱신, approval 질문 준비용 `TodoWrite`까지 차단될 수 있다.
- 승인 플래그가 workflow/session/phase 단위로 스코프되지 않아 오래된 승인 상태가 다음 workflow에 재사용될 수 있다.

Fix:
- `TodoWrite` input을 parse해서 `phase == execute` 전환 또는 execute checklist in-progress 전환 때만 approval을 요구한다.
- approval metadata key를 workflow id 또는 card started_at 기반으로 스코프한다.
- workflow clear/done/new init 때 approval flag를 제거한다.

Acceptance:
- plan phase에서 `TodoWrite`가 plan 유지 목적이면 허용되는 테스트.
- explicit approval 없이는 `phase=execute` 전환이 차단되는 테스트.
- 새 workflow 시작 시 이전 approval이 무효화되는 테스트.

### P0-4. 상태 변경 도구가 read-only로 등록되어 parallel-safe 취급될 수 있음

Evidence:
- `internal/tools/taskplaninit/taskplaninit.go:70`은 `IsReadOnly() bool { return true }`.
- 같은 tool은 `internal/tools/taskplaninit/taskplaninit.go:135-141`에서 `SetWorkflowCard`, `SetPendingWorkflowInit`, todostore save, state todos write를 수행한다.
- `internal/tools/enterplanmode/enterplanmode.go:34`와 `internal/tools/exitplanmode/exitplanmode.go:47`도 read-only로 표시되어 있지만 plan mode/state/file side effect가 있다.
- `internal/services/tools/orchestration.go:62-65`는 별도 evaluator가 없으면 `IsReadOnly()`를 concurrency-safe fallback으로 사용한다.

Impact:
- `task_plan_init`, `enter_plan_mode`, `exit_plan_mode`가 다른 read-only 도구와 병렬 실행될 수 있다.
- workflow/todo/plan 파일 상태와 UI 이벤트 순서가 race condition에 노출된다.

Claude reference:
- `claude_cli_참고용/src/Tool.ts:750-759`는 concurrency-safe 기본값을 false로 둔다.
- `claude_cli_참고용/src/services/tools/toolOrchestration.ts:97-108`은 input schema parse 성공 후 `isConcurrencySafe`를 try/catch로 평가한다.

Fix:
- permission read-only와 concurrency-safe를 분리한다.
- 상태를 쓰는 도구는 `IsReadOnly=false` 또는 `IsConcurrencySafe=false`로 명시한다.
- `pendingWorkflowInitBlocks`는 read-only 여부로 `task_plan_init` 강제 규칙을 우회하지 않게 한다.

Acceptance:
- `task_plan_init`, `enter_plan_mode`, `exit_plan_mode`가 parallel batch에 들어가지 않는 테스트.
- pending workflow init 상태에서 첫 tool이 `task_plan_init`이 아니면 read-only tool이어도 차단되는 테스트.

### P0-5. `WorkflowCard`가 deep-copy되지 않아 외부 mutation에 취약함

Evidence:
- `internal/state/workflow_state.go:39-49`는 전달받은 `*WorkflowCard`를 그대로 metadata에 저장한다.
- `internal/state/workflow_state.go:91-92`는 저장된 pointer를 그대로 반환한다.
- `internal/state/workflow_state.go:83-87`은 read lock을 풀고 write lock을 잡는 수동 upgrade 패턴을 사용한다.
- 현재 `TestSetWorkflowCardClonesConstraints`가 실패한다.

Impact:
- 호출자가 원본 `Constraints` slice를 수정하면 AppState 내부 값도 바뀐다.
- `WorkflowCard()` 반환값을 외부에서 수정하면 lock 없이 AppState가 변한다.
- session restore의 `map[string]interface{}` 변환 path가 lock upgrade로 복잡하고 취약하다.

Fix:
- `cloneWorkflowCard` helper를 만들고 `SetWorkflowCard`, `WorkflowCard`, `SetWorkflowPhase`에서 사용한다.
- `Constraints` slice는 always copy한다.
- map conversion은 lock 밖에서 변환 후 write lock으로 replace하거나, 반환만 clone하고 cache replace는 단순화한다.

Acceptance:
- 기존 failing test 통과.
- `WorkflowCard()` 반환값 mutate 후 재조회해도 저장값이 유지되는 테스트 추가.

## 3. P1 issues

### P1-1. TODO read/write source가 달라 UI, tool result, disk가 어긋날 수 있음

Evidence:
- `TodoWrite`는 `internal/tools/todowrite/todowrite.go:98-103`에서 disk를 읽되 AppState 값이 있으면 state를 우선한다.
- `TodoRead`는 `internal/tools/todoread/todoread.go:50`에서 todostore만 읽고 AppState를 보지 않는다.
- footer/state UI는 AppState의 todos를 사용한다.
- `internal/todostore/store.go:46-63`의 `NormalizeForStorage`는 empty/all-completed를 nil로 바꾼다.
- `internal/todostore/store.go:65-83`의 `SyncForSteps`는 steps 길이가 같으면 새 step 내용을 버리고 기존 content/active form만 보존하며 status를 pending으로 재설정한다.

Impact:
- state가 최신인데 disk가 stale이면 `TodoRead` 결과와 footer가 다를 수 있다.
- all-completed 저장 시 파일 내용이 JSON `null`이 되어, nil과 empty list의 의미가 모호해진다.
- 같은 길이의 새 plan을 승인하면 old content가 남아 실행 checklist가 틀어질 수 있다.

Claude reference:
- `claude_cli_참고용/src/utils/sessionRestore.ts`는 transcript의 마지막 TodoWrite를 복원 기준으로 삼는다.
- `claude_cli_참고용/src/utils/attachments.ts`는 TodoWrite가 오래 없으면 reminder를 붙이는 흐름이 있다.

Fix:
- `TodoRead`도 AppState 값을 우선하고, 없을 때 disk fallback을 사용한다.
- storage policy를 명확화한다. all-completed는 파일 삭제, `[]`, 또는 `null` 중 하나로 통일하고 tests에 박는다.
- `SyncForSteps`는 step identity를 비교한다. 같은 길이라도 tool/prompt가 바뀌면 content를 갱신한다.

Acceptance:
- state-only todos가 `TodoRead`에 보이는 테스트.
- 같은 길이의 steps 내용 변경 시 content가 갱신되는 테스트.
- all-completed persistence policy test.

### P1-2. Web evidence prompt와 WebSearch tool prompt가 서로 충돌함

Evidence:
- `internal/query/context.go:56`은 current/latest/명시적 검색에서 반드시 `web_search`와 `webfetch`를 호출하라고 지시한다.
- `internal/query/web_evidence.go:264-272`는 `webfetch` 성공 없이는 최종 답변을 막는다.
- `internal/tools/websearch/prompt.go:33-34`는 latest/current/news에서 snippet이 충분하면 webfetch를 생략하라고 지시한다.
- `internal/tools/websearch/websearch.go:182-183`의 실제 output message는 selected URL로 `webfetch`를 쓰라고 말한다.

Impact:
- 모델은 tool prompt 때문에 snippet-only로 답하려 하고, engine은 web evidence gate로 다시 막는 반복이 생긴다.
- retry prompt 품질이 떨어지고 token/latency가 낭비된다.

Fix:
- WebSearch prompt에서 current/latest/news는 `webfetch` 필수로 정렬한다.
- snippet-only 허용은 low-stakes static query 등 web evidence policy disabled인 경우로 한정한다.
- `web_evidence.go`의 tool-name observe도 canonical helper를 사용한다.

Acceptance:
- current/latest 질의에서 `web_search` 후 `webfetch` 없이 final을 시도하면 follow-up이 web-specific으로 나가는 테스트.
- WebSearch prompt snapshot test.

### P1-3. Web evidence retry보다 generic "use tools" retry가 먼저 실행됨

Evidence:
- `internal/query/engine.go:850-861`은 첫 iteration에서 text-only 응답이면 generic force message를 먼저 넣는다.
- `internal/query/engine.go:862-868`의 web evidence continuation은 그 다음이다.

Impact:
- current/latest 질문인데 첫 retry가 `bash`, `powershell`, `server_copy` 등을 쓰라는 generic 문구로 나가 web path를 흐릴 수 있다.

Fix:
- `webPolicy.Enabled`이고 evidence 부족이면 generic force보다 web evidence follow-up을 우선한다.
- generic force message는 task execution 전용으로 한정하고, greeting/chit-chat 예외와 충돌하지 않게 한다.

Acceptance:
- web-required text-only 응답에서 첫 follow-up이 `web_search`/`webfetch`를 요구하는 테스트.

### P1-4. Parallel orchestration 보호 장치가 Claude보다 부족함

Evidence:
- Argus `internal/services/tools/orchestration.go:35-65`는 schema validation 없이 concurrency-safe를 평가하고, evaluator panic recover가 없다.
- `internal/services/tools/orchestration.go:92-101`는 `WaitGroup`으로 batch 전체를 기다리며 max concurrency cap이 없다.
- Claude `toolOrchestration.ts:8-10`은 default concurrency cap 10을 둔다.
- Claude `toolOrchestration.ts:97-108`은 input schema parse와 try/catch를 거친다.
- Claude `StreamingToolExecutor.ts:129-148`은 streaming 중 tool을 즉시 큐잉/실행하고 unsafe tool 순서를 보장한다.
- Claude `StreamingToolExecutor.ts:357-362`는 Bash error 시 sibling Bash를 abort하되 read/webfetch는 독립으로 둔다.
- Claude `toolOrchestration.ts:31-58`은 concurrent context modifiers를 queue 후 batch 뒤에 적용한다.

Impact:
- 많은 read-only calls가 한 번에 실행되어 리소스를 과점할 수 있다.
- malformed input 또는 evaluator panic이 orchestration 전체를 깨뜨릴 수 있다.
- streaming 중 tool 실행이 시작되지 않아 latency가 Claude 구조보다 길다.
- hook/context mutation이 parallel goroutine 안에서 일어나면 race risk가 있다.

Fix:
- `ARGUS_MAX_TOOL_CONCURRENCY` 또는 config default 10 cap 추가.
- tool input schema validation helper를 공통화하고, invalid input은 concurrency-unsafe로 처리한다.
- `IsConcurrencySafe` 호출을 recover로 감싼다.
- context/state modifier를 반환하는 hook은 parallel batch 내에서 바로 적용하지 않고 queue 후 ordered apply한다.
- streaming executor 도입은 별도 큰 작업으로 분리하되, 우선 batch executor 안정화부터 한다.

Acceptance:
- invalid input, panic evaluator, cap limit, ordered results 테스트.
- Bash-like command failure 시 sibling cancellation 정책 테스트.

### P1-5. Parallel result ordering과 graph append ordering이 달라질 수 있음

Evidence:
- engine은 `internal/query/engine.go:903-909`에서 parallel batch UI event를 먼저 낸다.
- `RunToolsWithDispatcher`는 결과 slice를 call order로 채우지만, `invokeTool` distiller path는 `internal/query/engine.go:1227-1229`에서 goroutine completion order로 graph에 append한다.
- legacy message append는 `internal/query/engine.go:932`에서 returned result order를 따른다.

Impact:
- LLM graph context와 legacy messages의 tool result order가 달라질 수 있다.
- parallel web/search/file 결과가 completion order로 들어가면 다음 iteration의 reasoning context가 비결정적일 수 있다.

Fix:
- `invokeTool`은 distilled result를 반환만 하고 graph append는 orchestration result order loop에서 한다.
- 또는 parallel batch result buffer를 둬서 call order로 graph append를 수행한다.

Acceptance:
- 느린 tool과 빠른 tool을 섞은 parallel batch에서 graph result order가 call order로 유지되는 테스트.

### P1-6. Shell read-only 판정이 과도하게 낙관적임

Evidence:
- `internal/tools/permission_policy.go:52-56`에서 `find`, `gh`, `python`, `python3`가 read-only command map에 포함된다.
- `internal/tools/permission_policy.go:296-298`은 `gh repo`, `gh workflow`, `gh release` 등을 subcommand 단위로 read-only 취급한다.
- `internal/tools/bash/command_semantics.go:23`도 `find`를 read-only regex에 포함한다.
- Bash/PowerShell tool의 `IsConcurrencySafe`는 `internal/tools/bash/bash.go:94-96`, `internal/tools/powershell/powershell.go:104-106`에서 이 policy에 의존한다.

Impact:
- `python -c` 파일 쓰기, `find -delete`, 일부 `gh repo` mutation 계열이 read-only/parallel-safe로 오판될 수 있다.

Fix:
- Python은 기본 unsafe로 돌리고, 명시적으로 허용된 read-only script/flags만 예외 처리한다.
- `find`는 `-delete`, `-exec`, `-ok`, redirection, command substitution 등을 검사한다.
- `gh`는 command+subcommand+flags matrix로 좁힌다. 예: `gh pr view/list/status`, `gh issue view/list`, `gh repo view`만 우선 허용.

Acceptance:
- unsafe shell examples가 concurrency-safe false로 판정되는 tests.

### P1-7. TUI parallel grouping lookup과 renderer name normalization 보강 필요

Evidence:
- `internal/presentation/events.go:295-296`은 `BatchTaskIDs`를 comma-joined string으로 `Event.Input`에 담는다.
- `internal/tui/transcript.go:342-345`는 comma split으로 batch IDs를 복원한다.
- `internal/tui/transcript.go:897-901`은 child lookup을 parent index로 저장한다.
- `internal/tui/transcript.go:767-813`의 `rebuildToolEntryByTaskID`는 top-level lookup만 재구성하며 parallel sub lookup은 별도 검증이 필요하다.
- `internal/tui/toolui/renderer.go:72-73`은 renderer lookup을 exact key로 수행한다.
- `internal/tui/transcript.go:600,614`는 `web_fetch`만 collapsible/read-style 분류에 포함하고 실제 `webfetch`는 빠져 있다.

Impact:
- trimming/rebuild 이후 parallel child result/delta routing이 stale index에 걸릴 수 있다.
- alias tool name이 들어오면 inline label은 맞지만 renderer는 못 찾을 수 있다.
- 실제 `webfetch`가 collapsible/read-style classification에서 빠져 UI behavior가 alias와 다를 수 있다.

Fix:
- presentation Event에 `BatchTaskIDs []string` 같은 typed field를 추가하거나 input encoding을 JSON으로 바꾼다.
- `rebuildToolEntryByTaskID`에서 `parallelSubLookup`도 재구성한다.
- toolui renderer registry를 canonical name 기반으로 조회한다.
- `webfetch`, `web_search` classification을 alias와 함께 명시한다.

Acceptance:
- parallel group 생성, trim 후 result routing, view clear 후 lookup reset 테스트.
- `webfetch`와 `web_fetch`가 같은 renderer/classification을 받는 테스트.

## 4. P2 issues

### P2-1. WebSearch UI가 string result message를 잃음

Evidence:
- `internal/tools/websearch/websearch.go:156`과 `180`은 no-result guidance를 string entry로 넣는다.
- `internal/tools/websearch/ui.go:95-99`는 `map[string]any`가 아닌 result entry를 무시한다.
- `internal/tools/websearch/ui.go:51-52`는 결국 generic `No results found`만 표시한다.

Impact:
- "broader search terms" 같은 구체적 retry guidance가 UI에서 사라진다.

Fix:
- `WebSearchInteractiveModel`에 `messageLines []string`을 추가해 string result를 표시한다.
- success summary string은 중복 표시를 피하되 no-result/diagnostic string은 보존한다.

### P2-2. WebSearch provider routing과 domain filters 적용 순서 점검 필요

Evidence:
- `internal/tools/websearch/websearch.go:136-145`에서 provider route가 먼저 선택된다.
- `internal/tools/websearch/websearch.go:173`에서 allowed/blocked domain filter는 search 후 적용된다.

Impact:
- `allowed_domains`가 provider domain과 맞지 않으면 provider를 호출한 뒤 filtered empty가 되고, fallback 여부가 애매해진다.

Fix:
- provider 선택 전에 allowed domain과 provider domain compatibility를 확인한다.
- filtering 후 empty가 된 경우 weak route라면 SearXNG fallback을 허용한다.

### P2-3. WebFetch permission default가 Claude와 다름

Evidence:
- Claude `WebFetchTool.ts:123-177`은 host-specific allow/ask/deny rule이 없으면 ask를 반환한다.
- Argus `internal/tools/webfetch/webfetch.go:264` 주변 policy는 webfetch가 read-only라 기본 allow에 가깝다.

Impact:
- 의도된 UX일 수 있지만, Claude parity 관점에서는 보안/프라이버시 기본값이 다르다.

Fix:
- 제품 의도 결정 필요. Claude parity가 목표라면 unknown host는 ask, trusted/preapproved host만 allow로 바꾼다.
- current system behavior 유지가 목표라면 `modify.md` 이후 별도 design note에 명시한다.

### P2-4. AIDebug event coverage에 `UIEventParallelBatch` 누락 가능성

Evidence:
- `internal/query/stream.go:24`에 `UIEventParallelBatch`가 추가되어 있다.
- `cmd/argus/main_aidebug_test.go`의 all event coverage 목록은 별도 확인 및 갱신이 필요하다.

Impact:
- 전체 테스트에서 새 UI event kind가 coverage test에 걸릴 수 있다.

Fix:
- aidebug event allow/coverage list에 `parallel_batch`를 추가한다.

### P2-5. Session snapshot metadata clone 필요

Evidence:
- AppState `Metadata`는 map이다.
- session snapshot 저장 path에서 metadata map을 직접 참조하면 저장 중 mutation/race 가능성이 있다.

Impact:
- concurrent tool/state updates 중 snapshot이 inconsistent하게 저장될 수 있다.

Fix:
- AppState에 `MetadataSnapshot()`을 추가해 lock 안에서 shallow/deep copy를 반환한다.
- workflow card/todo 관련 nested values는 clone helper를 재사용한다.

### P2-6. 일부 파일 주석 encoding/mojibake 정리

Evidence:
- `internal/tools/shell_validation.go` 등에서 PowerShell 출력 기준 mojibake가 보인다.

Impact:
- 빌드에는 직접 영향이 없을 수 있지만 유지보수성이 떨어진다.

Fix:
- 파일 encoding이 실제로 UTF-8인지 확인하고, 깨진 주석은 한국어 또는 영어로 재작성한다.
- 기능 변경 PR과 분리하는 것이 좋다.

## 5. Working tree and HEAD comparison notes

현재 작업트리는 매우 dirty 상태이며 `.Argus` session/artifact/traces 삭제가 대량으로 잡혀 있다. 이번 감사/문서 작성에서는 해당 변경을 되돌리거나 정리하지 않았다.

현재 작업트리에서 특히 중요한 신규/변경 파일:
- `internal/query/workflow.go`, `internal/query/workflow_test.go`: workflow phase gate와 heuristic 테스트.
- `internal/state/workflow_state.go`, `internal/state/workflow_state_test.go`: workflow card state와 deep-copy 실패 테스트.
- `internal/tools/taskplaninit/`: 6-phase plan initialization tool 및 phase guide.
- `internal/services/tools/orchestration.go`: `ToolBatch` export 및 input-aware concurrency evaluator.
- `internal/tui/transcript.go`, `internal/presentation/events.go`: parallel batch UI grouping.

HEAD 대비 current에서 좋은 방향으로 들어온 점:
- tool orchestration에 input-aware `IsConcurrencySafe(input)` hook이 도입되어 shell command별 parallel 판정의 기반이 생겼다.
- engine에서 `UIEventParallelBatch`를 발행하고 TUI에서 parallel group으로 묶는 흐름이 생겼다.
- web evidence gate가 `web_search` + `webfetch` 조합을 강제하려는 방향은 hallucination 방지에 맞다.

HEAD 대비 current에서 새로 생긴 주요 리스크:
- workflow 정책이 `taskplaninit`과 `query/workflow.go` 두 군데로 분리되어 즉시 drift가 생겼다.
- 상태를 쓰는 신규 tool이 read-only로 표시되어 parallel engine의 safety model과 충돌한다.
- 신규 workflow state가 pointer/slice clone 없이 AppState metadata에 저장된다.

## 6. Recommended implementation order

1. Canonical tool names와 phase allow-list 단일화
   - `taskplaninit.PhaseAllowedTools`를 enforcement source로 사용.
   - `webfetch`/`web_fetch` alias 정리.
   - phase guide/enforcement coverage test 추가.

2. Workflow/TODO state correctness
   - `WorkflowCard` clone helper 추가.
   - approval gate를 `TodoWrite` input-aware로 변경.
   - approval metadata lifecycle 정리.
   - `TodoRead` state-first fallback-disk로 변경.

3. Read-only와 concurrency-safe 분리
   - state-changing tools는 concurrency unsafe.
   - orchestration에 schema validation, panic recovery, concurrency cap 추가.
   - shell read-only allow-list 축소.

4. Web evidence와 WebSearch prompt 정렬
   - latest/current prompt contradiction 제거.
   - web evidence follow-up 우선순위 조정.
   - web search provider/domain filtering behavior 테스트.

5. TUI parallel UI hardening
   - renderer/collapsible canonicalization.
   - parallel lookup rebuild 테스트.
   - typed batch event 또는 safe encoding 도입.

6. Lower-priority cleanup
   - AIDebug event coverage.
   - metadata snapshot clone.
   - mojibake comments cleanup.

## 7. Test plan

Minimum targeted tests after fixes:

```text
go test ./internal/query ./internal/state ./internal/services/tools
go test ./internal/tools/todowrite ./internal/tools/todoread ./internal/todostore
go test ./internal/tools/websearch ./internal/tools/webfetch
go test ./internal/tui ./internal/presentation
```

Full validation:

```text
$env:GOCACHE='C:\Dev\Argus\.gocache'
go test ./...
```

Required new/updated test cases:
- research phase accepts `webfetch` and alias `web_fetch`.
- phase guide list and enforcement list stay in sync.
- plan phase `TodoWrite` is allowed unless it advances to execute.
- execute transition requires scoped explicit approval.
- `WorkflowCard` set/get deep-copy protects `Constraints`.
- `TodoRead` prefers AppState over disk when state has current todos.
- state-mutating tools are not parallel batched.
- invalid/panic concurrency evaluator falls back to unsafe.
- parallel graph append preserves tool-call order.
- current/latest text-only response receives web-specific follow-up first.
- WebSearch UI preserves no-result guidance strings.
- TUI parallel group lookup survives trim/rebuild and clear.

## 8. Implementation status (2026-04-28)

### 8.1 Completed changes

- Added a canonical tool-name layer.
  - `internal/tools.CanonicalName` is now shared by registry lookup, tool UI renderer lookup, workflow enforcement, web evidence tracking, and transcript classification.
  - Normalized aliases include `web_fetch -> webfetch`, `websearch -> web_search`, `file_read/read -> fileread`, `mcp_tool -> mcp`, `todo_read/todoread`, `todo_write/todowrite`, and `askuser/ask_user_batch -> ask_user`.

- Hardened workflow state and approval lifecycle.
  - `WorkflowCard()` returns a detached copy instead of exposing map/slice internals.
  - Replacing or clearing the workflow card clears workflow approval.
  - Workflow approval now uses scoped AppState helpers instead of a raw global `user_approved` metadata flag.
  - Session persistence now snapshots cloned metadata instead of reading the mutable metadata map directly.

- Reworked workflow phase enforcement.
  - Pending workflow initialization blocks every tool except `task_plan_init`.
  - Phase allow-lists use `taskplaninit.PhaseAllowedTools` as the single source of truth.
  - Plan-phase `TodoWrite` is blocked only when the input advances to `{"phase":"execute"}` and approval is missing.
  - `task_plan_init`, `enter_plan_mode`, and `exit_plan_mode` are explicitly concurrency-unsafe.

- Fixed TODO state/read synchronization.
  - `TodoRead` now prefers `ctx.State.Todos(sessionID)` and only falls back to disk when state is empty.
  - `todostore.SyncForSteps` rebuilds stale TODO content even when the list length is unchanged.
  - User-edited active TODO form is preserved when the generated step content is unchanged.

- Improved parallel execution and added streaming-ready scheduling.
  - Batch execution has an `ARGUS_MAX_TOOL_CONCURRENCY` cap, defaulting to 10.
  - `IsConcurrencySafe` is protected by basic JSON-shape validation and panic recovery; invalid or panicking evaluators are treated as unsafe.
  - Added `StreamingToolExecutor`, which can start safe tools at `tool_use_start` time, preserves final result order, serializes unsafe tools, and cancels sibling shell commands on shell failure.
  - Query engine now uses the streaming executor and appends graph tool results in original tool-call order.

- Tightened shell read-only policy.
  - Removed `python/python3` from read-only command allow-list.
  - `find -delete`, `find -exec*`, and `find -ok*` are treated as mutating.
  - `gh` read-only handling is limited to known read commands such as `repo view`, `issue list/view`, `pr list/view`, and `api GET`.
  - Cleaned up mojibake in shell validation comments/messages.

- Improved web-search and web-fetch consistency.
  - Latest/current/news prompt policy now states that `web_search` is discovery only and `webfetch` verification is required before final answers.
  - Provider routing skips providers incompatible with `allowed_domains`.
  - WebSearch UI now preserves no-result or diagnostic string messages.
  - Web evidence tracking canonicalizes `web_search` and `webfetch` names.

- Hardened TUI and presentation event links.
  - `UIEventParallelBatch` now carries typed `BatchTaskIDs`; comma-joined `Input` remains as compatibility fallback.
  - Transcript rebuild also reconstructs `parallel_group` sub-lookups.
  - Tool collapsible/classification logic uses canonical names.
  - AI debug event coverage includes `UIEventParallelBatch`.

### 8.2 Tests added or updated

- `internal/query/workflow_test.go`
  - Pending workflow-init blocking.
  - Research phase `webfetch` alias acceptance.
  - Execute-phase `TodoWrite` approval boundary.

- `internal/state/workflow_state_test.go`
  - Detached `WorkflowCard()` copy behavior.

- `internal/services/tools/orchestration_test.go`
  - Invalid input and panic evaluator downgrade to unsafe.
  - Plan-mode tools are concurrency-unsafe.
  - Concurrency cap enforcement.
  - Streaming executor early start, order preservation, and unsafe blocking.

- `internal/tools/bash/concurrency_test.go`
  - Python unsafe.
  - Mutating `find` flags unsafe.
  - `gh` read/write subcommand distinction.

- `internal/tools/todoread/todoread_test.go`
  - State-first read and disk fallback.

- `internal/todostore/store_test.go`
  - Same-length stale content rebuild and active form preservation.

- `internal/tools/websearch/ui_test.go`
  - Diagnostic/no-result string fallback rendering.

### 8.3 Validation result

Targeted suites passed:

```powershell
$env:GOCACHE='C:\Dev\Argus\.gocache'
go test ./internal/query ./internal/state ./internal/services/tools ./internal/tools/todoread ./internal/todostore ./internal/tools/websearch ./internal/tools/bash ./internal/tools/powershell ./internal/presentation ./internal/tui ./cmd/argus
go test ./internal/tools/taskplaninit ./internal/tools/todowrite ./internal/tools/webfetch ./internal/tools
```

Full suite command:

```powershell
$env:GOCACHE='C:\Dev\Argus\.gocache'
go test ./...
```

The full suite still fails in areas outside this implementation scope and already dirty in the working tree:

- `internal/repl/commands`
  - `TestServerDisconnectWithoutAliasResetsActiveWorkspace`: active workspace remains `a100-server` instead of returning to `local`.
- `internal/services/workspace`
  - `TestManagerCopyFileOverwriteFalse`: overwrite=false protection error is not returned.
  - `TestManagerListDirDepth`: expected `level1` entry is missing from depth listing.
- `internal/tools/servercopy`
  - `servercopy_test.go` still references `parseCopyRequest`, but the current dirty `servercopy.go` no longer defines it.

These remaining failures should be handled as a separate workspace/servercopy cleanup before expecting `go test ./...` to be green.
