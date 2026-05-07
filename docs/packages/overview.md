# 패키지 구조 개요

```
github.com/koreaf16/argus
├── cmd/argus/                    # 진입점 — 플래그, 부트스트랩, 모드 라우팅
├── internal/
│   ├── aidebug/                  # --aidebug 모드용 NDJSON sink
│   ├── components/               # UI 컴포넌트 (logo, promptinput)
│   ├── constants/                # 상수 및 디렉토리 경로
│   ├── context/                  # Episodic Context Graph 시스템
│   ├── hooks/                    # 이벤트 기반 훅 시스템
│   ├── memdir/                   # ~/.argus 디렉토리 bootstrap 및 JSON 영속화
│   ├── planmode/                 # 계획 모드 및 plans 관리
│   ├── presentation/             # 프레젠테이션 이벤트 및 footer 상태
│   ├── query/                    # 쿼리 엔진 (핵심)
│   ├── redact/                   # 민감 정보 마스킹
│   ├── repl/                     # REPL 루프 및 슬래시 커맨드
│   ├── security/                 # 보안 관련 유틸리티
│   ├── services/                 # 외부 서비스 연동
│   │   ├── llm/                  # LLM 인터페이스 및 3 공급자
│   │   ├── mcp/                  # MCP 설정 로드 및 브리지
│   │   ├── lsp/                  # Language Server Protocol
│   │   ├── tools/                # Tool orchestration + HookRegistry
│   │   └── workspace/            # SSH 기반 원격 워크스페이스
│   ├── session/                  # 세션 ID 생성 및 Snapshot 관리
│   ├── shelljobs/                # 백그라운드 셸 작업 관리
│   ├── skills/                   # Skills 레지스트리
│   ├── state/                    # 앱 상태 관리
│   ├── tasks/                    # 태스크(Task) 관리 및 영속화
│   ├── tools/                    # Tool 시스템
│   ├── tui/                      # BubbleTea 기반 TUI
│   ├── types/                    # 공통 타입 정의
│   └── utils/                    # 유틸리티
└── docs/                         # 이 문서들
```

## 책임 매트릭스

| 계층 | 패키지 | 책임 |
|------|--------|------|
| **진입점** | `cmd/argus` | 플래그 파싱, 부트스트랩, 모드 라우팅 |
| **프레젠테이션** | `tui`, `repl`, `components` | 사용자 인터페이스 |
| **오케스트레이션** | `query`, `hooks` | ReAct 루프, 훅 디스패치 |
| **컨텍스트** | `context`, `state` | Graph, Distiller, AppState |
| **도구** | `tools`, `services/mcp` | 내장 도구 + MCP 브리지 |
| **서비스** | `services/llm`, `services/lsp`, `services/workspace` | 외부 시스템 연동 |
| **유틸리티** | `utils/`, `types`, `constants` | 공통 헬퍼, 타입, 상수 |

## 의존성 그래프 (요약)

```
cmd/argus
  ├── tui → query, tools, hooks, state, session
  ├── repl → query, tools, state
  │
query.Engine
  ├── context (Graph, Distiller, Renderer)
  ├── tools (Registry, Tool)
  ├── services/llm (LLM interface)
  ├── hooks (Dispatcher)
  ├── state (AppState)
  └── types
  │
context.Graph
  ├── context.Distiller → context.ArtifactStore
  └── services/llm (Message projection)
  │
tools.Registry
  ├── services/mcp (BridgeTool)
  └── types
  │
services/llm
  ├── anthropic.go, gemini.go, openai.go
  └── types
  │
hooks.Dispatcher
  ├── hooks.Executor
  └── types
```
