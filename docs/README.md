# Argus Documentation

Argus 프로젝트의 기술 문서 및 가이드라인입니다. 모든 문서는 최신 구현 원칙과 아키텍처를 기준으로 유지 관리됩니다.

## 📂 문서 구조

### 1. [Architecture](./architecture/overview.md)
- 시스템의 핵심 아키텍처와 런타임 흐름을 설명합니다.
- **Episodic Context Graph**: 대화 문맥 관리 및 토큰 최적화 전략.
- **Component Layout**: 각 패키지의 역할과 의존 관계.

### 2. Features (기능 가이드)
- **[LLM Providers](./features/llm-providers.md)**: 지원되는 LLM 서버(OpenAI, Anthropic, Gemini) 설정 및 관리.
- **Tools**: 내장 도구 및 MCP 기반 확장 도구 가이드.
  - [Web Search](./features/tools/websearch.md)
- **Terminal Integration**: 터미널 특화 기능 가이드.
  - [IME Cursor Parking](./features/terminal/ime-cursor-parking.md)

### 3. Planning (프로젝트 계획)
- **Phases**: 프로젝트 단계별 로드맵.
  - [Phase 1: Foundation](./planning/phases/phase1.md)
  - [Phase 2: Advanced Tools](./planning/phases/phase2.md)
- **[UI/UX Strategy](./planning/ui-ux/conversion-plan.md)**: Claude 스타일 UI 구현 전략 및 "Dynamic Inline Rendering" 원칙.

### 4. Development (개발 기록)
- **[CHANGELOG](./development/CHANGELOG.md)**: 버전별 변경 이력.
- **Test Reports**: 주요 테스트 결과 및 리포트.
  - [Latest Report](./development/test-reports/latest.md)

---

## 🛠️ 핵심 개발 원칙 (Quick Link)
상세한 구현 규칙은 프로젝트 루트의 [agent.md](../agent.md)를 참조하십시오.
- **UI**: AltScreen 모드 금지, Dynamic Inline Rendering 필수.
- **Language**: Go 중심 구현 (TS 코드 포팅 시 Go 관례 준수).
- **Communication**: 보고 및 계획은 반드시 한국어로 수행.
