# UI/UX Strategy: Claude-style Dynamic Rendering

Argus의 UI는 Claude Code의 사용자 경험을 지향하지만, 터미널의 기본 동작 방식을 존중합니다.

## 🏁 핵심 원칙: AltScreen 금지 (No Fullscreen TUI)
대부분의 TUI 도구들이 사용하는 `AltScreen` 모드(전체 화면을 차지하고 종료 시 이전 내용이 사라지는 방식)를 사용하지 않습니다.

1. **Terminal Scrollback 유지**: 모든 답변은 터미널 스크롤백의 일부가 되어야 합니다.
2. **Dynamic Inline Rendering**: 하단에 상태바(Footer)와 입력창(Input)을 고정하되, 답변 내용은 위로 흐르듯이(Scrolling) 출력됩니다.
3. **UX Continuity**: 사용자가 이전 대화 내용을 터미널 스크롤로 언제든 찾아볼 수 있어야 합니다.

## 🏗️ UI 구조
- **Transcript Area**: 답변이 출력되는 영역. `tea.Println` 등을 활용해 스크롤백에 직접 씁니다.
- **Fixed Anchor Area**: 터미널 하단에 고정된 영역.
  - **Prompt Input**: 멀티라인 지원 입력창.
  - **Footer/Status Bar**: 현재 모델, 권한 모드, 작업 상태 등을 표시.

## 🛠️ 구현 단계
1. **1단계 (기초)**: `Bubble Tea` 기반의 기초 레이아웃 구성 및 Inline 렌더링 엔진 구축.
2. **2단계 (고급)**: 멀티라인 입력 처리 개선 및 툴 실행 상태의 실시간 인라인 업데이트.
3. **3단계 (완성)**: 권한 승인 모달(Inline Overlay) 및 단축키 바 완성.

## 🔍 참고 대상
- `claude_cli_참고용/`: Claude Code의 React/Ink 기반 구조를 Go로 재해석하여 적용.
- `gemini_cli_참고용/`: Gemini CLI의 출력 방식 참조.
