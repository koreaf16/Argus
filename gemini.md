# Argus Gemini 작업 지침

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

## 도구(Tool) 명명 규칙
- **`Name()` 반환값은 반드시 `snake_case`** (소문자 + 언더스코어). `"my_tool"` ✅ / `"MyTool"` ❌
- 대소문자 불일치 시 `CanonicalName()`, `safeAutoModeTools`, `transcript.go` 등에서 조용히 실패함
- 새 도구 추가 시 반드시: ① `cmd/argus/main.go` 등록 ② `classifier_decision.go` `safeAutoModeTools` 추가 ③ `canonical.go` alias (필요 시)
- 상세 체크리스트는 `agent.md` → "도구(Tool) 명명 규칙" 섹션 참조

## 상세 규칙 참조
- 구현/아키텍처/문서 동기화 규칙은 `agent.md`를 기준으로 따릅니다.
