# Argus Gemini 작업 지침

## 커뮤니케이션 규칙
- 모든 보고와 사전 계획은 반드시 한국어로 작성합니다.
- 사용자 응답은 반드시 존댓말로 작성합니다.

## 프로젝트 기본 원칙
- 별도 지시가 없으면 모든 작업 대상은 `Argus.exe` 프로젝트입니다.
- UI/동작 구현은 `C:\Dev\Argus\claude_cli_참고용` 구조를 우선 기준으로 맞춥니다.
- **UI 구현 시 `AltScreen`(fullscreen TUI) 모드를 사용하지 않습니다. 터미널 스크롤백이 유지되면서 하단 상태바가 고정되고 답변이 흐르듯이 출력되는 Dynamic Inline Rendering 방식을 사용해야 합니다. (gemini_cli / claude_cli 방식)**
- 연속 실행/검증 시 `session_id`와 `--resume`(`-r`)를 사용해 세션 맥락을 유지합니다.
- 자동화 테스트성 출력이 필요할 때는 `--aidebug`를 사용합니다.

## 상세 규칙 참조
- 구현/아키텍처/문서 동기화 규칙은 `agent.md`를 기준으로 따릅니다.
