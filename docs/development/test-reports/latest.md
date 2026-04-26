# Argus 통합 테스트 및 파괴적 스트레스 테스트 리포트

- **테스트 일시**: 2026-04-22
- **테스트 환경**: Win32, Argus (Go version 1.22+), Sandbox Server (192.168.0.130)
- **활성 모델**: gemma-4-26b (OpenAI-compat)

## 1. 테스트 요약
| 영역 | 상태 | 비고 |
| :--- | :--- | :--- |
| **Bash Tool (SSH)** | **실패** | sshpass 부재 및 PTY 미지원으로 인한 비밀번호 주입 실패 |
| **File/Glob/Grep Tool** | **성공** | 로컬 100개 파일 생성 및 검색 확인 (정확도 높음) |
| **Stress Test (CPU/Mem)** | **부분 성공** | 실행은 되나, 부모 프로세스 종료 시 자식 프로세스 정리 안 됨 |
| **Plan Mode** | **미흡** | 현재 스터브(Stub) 상태로 실제 상태 제어 로직 부재 |
| **Web Search/Fetch** | **성공** | 실시간 검색 및 결과 분석 기능 정상 작동 |
| **LSP/MCP** | **확인 중** | 코드상 구현되어 있으나 상세 연동 테스트 필요 |

## 2. 발견된 문제점 (Issues)
| ID | 요약 | 상세 내용 | 상태 |
| :--- | :--- | :--- | :--- |
| ISSUE-01 | sshpass 부재 및 SSH 연동 실패 | sshpass가 설치되지 않은 환경에서 fallback 시도 중 비밀번호 주입 안 되어 타임아웃. | open |
| ISSUE-02 | **하드코딩된 자격 증명 (보안)** | `internal/tools/bash/bash.go`에 특정 서버(192.168.0.130) 비밀번호가 평문 하드코딩됨. | **Critical** |
| ISSUE-03 | PTY 시뮬레이션 미구현 | `RunCommand`에서 `usePTY=true`를 받아도 실제 PTY(creack/pty 등)를 사용하지 않아 대화형 프롬프트 처리 불가. | open |
| ISSUE-05 | 좀비 프로세스 발생 | Argus 종료 시 `bash`가 띄운 하위 프로세스(예: `yes`)가 종료되지 않고 남음. | open |
| ISSUE-06 | 플랜 모드 도구 스터브 상태 | `EnterPlanMode` 도구가 실제 엔진 상태를 변경하지 않는 단순 출력용 스터브임. | open |

## 3. 테스트 상세 로그
### 3.1 도구 집중 검증 및 샌드박스 파괴적 테스트
- [X] 샌드박스 서버 SSH 연동 확인
  - 실패: `sshpass` 명령어를 찾을 수 없으며, 일반 `ssh` 실행 시 비밀번호 주입 도우미가 TTY 부재로 작동하지 않음.
- [X] 파일 시스템 파괴 및 탐색 테스트 (로컬)
  - 성공: 100개 파일 생성 및 `glob` 검색 확인. `grep`의 경우 제로 패딩 등 미세한 문자열 불일치 시 검색 안 됨(정상 동작).
- [X] 리소스 고갈 및 프로세스 제어 테스트
  - 실패: `yes > /dev/null` 실행 후 `argus.exe`를 강제 종료해도 `yes.exe`가 계속 실행됨 (PGID 관리 미흡).

### 3.2 워크플로우 및 플랜 모드
- [ ] 다단계 도구 연동 계획 실행 확인
  - 미흡: 플랜 모드 진입 메시지는 출력되나, 실제로 쓰기 도구를 차단하거나 상태를 유지하는 기능이 동작하지 않음.

### 3.3 기타 핵심 도구
- [X] Web Search & Fetch 연동 확인
  - 성공: "오늘 서울 날씨" 검색 시 다수의 웹 사이트 정보를 요약하여 보고함.
- [ ] LSP/MCP 연동 확인
  - `internal/services/lsp` 및 `internal/tools/lsptool` 구현 확인. 실제 언어 서버 연동 테스트 필요.
---

## 2026-04-22 Update (Plan Multi-Step Execution)
- [x] Plan mode state transition is wired (`EnterPlanMode`/`ExitPlanMode`).
- [x] `ExitPlanMode` now returns normalized `allowed_prompts` and writes numbered approved steps to plan file.
- [x] Engine emits `plan_execution_ready` and supports direct planned-step execution for `bash`/`powershell`.
- [x] REPL sequential runner executes approved steps with per-step confirmation and immediate stop-on-failure.
- [x] Todo status is synchronized per session during step execution (`pending` -> `in_progress` -> `completed`).
