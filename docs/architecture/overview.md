# Argus Architecture Overview

Argus는 Go 기반의 터미널 중심 AI 에이전트로, 효율적인 문맥 관리와 확장 가능한 도구 시스템을 특징으로 합니다.

## 🚀 런타임 흐름 (Runtime Flow)
1. **Bootstrapping**: `cmd/argus/main.go`가 모델 레지스트리, 앱 상태, MCP/LSP/Skills 매니저를 초기화합니다.
2. **TUI Initialization**: `tui.Run`이 Dynamic Inline Rendering 기반의 인터페이스를 실행합니다.
3. **Query Orchestration**: `query.Engine`이 LLM 스트림과 도구 호출 루프를 제어합니다.
4. **Context Management**: 모든 대화는 `internal/context`의 Episodic Context Graph에 기록됩니다.

## 🧠 Episodic Context Graph
단순 선형 기록이 아닌 그래프 구조로 문맥을 관리하여 토큰 비용을 최소화하고 검색 효율을 높입니다.
- **Node 종류**: User, Assistant, ToolUse, ToolResult, System, Notice.
- **Distiller**: 출력 데이터 크기에 따라 FULL, PARTIAL, SUMMARY로 자동 압축.
- **Budget aware**: 모델별 컨텍스트 윈도우의 20%를 출력을 위해 항상 예약.

## 📦 패키지 구조
- `internal/context`: 그래프 기반 문맥 관리, 토큰 추정, 아티팩트 저장.
- `internal/query`: 대화 루프 및 도구 실행 제어.
- `internal/tool`: 기본 도구 및 MCP 동적 브릿지 레지스트리.
- `internal/tui`: Bubble Tea 기반의 스크롤백 유지형 UI 엔진.
- `internal/services/mcp`: 외부 MCP 서버 연동 (JSON-RPC).
- `internal/services/lsp`: 언어 서버 연동을 통한 코드 분석.

## 🛠️ 도구 시스템
- **Built-in**: `websearch`, `fileread`, `filewrite`, `glob`, `grep` 등.
- **MCP Dynamic Bridge**: `mcp.json` 설정을 통해 런타임에 도구를 동적으로 등록.
- **Skills**: 복합적인 작업을 수행하는 고수준 시나리오 기반 도구.
