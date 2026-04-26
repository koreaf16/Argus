# REPL Scenario Testing Guide

## 목적

시나리오 테스트에서 `bin/infractl.exe --repl --debug --json`를 **세션 유지 상태**로 실행하고, 입력 주입과 응답 로그를 반복 검증하기 위한 운영 절차를 정의한다.

이 가이드는 일반 작업이 아니라, 아래 조건에서만 적용한다.

- 사용자 요청에 `시나리오`, `시나리오 테스트`, `REPL`, `세션 유지`, `--repl --debug --json`가 포함된 경우

## 표준 실행 흐름

1. 세션 시작

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File bin/repl_json_session_start.ps1 -ResetLogs
```

2. 상태 확인

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File bin/repl_json_session_status.ps1
```

기대 상태:

- `STATUS=running`
- `BROKER_ALIVE=True`
- `INFRACTL_PID` 값 존재

3. 입력 주입

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File bin/repl_json_session_send.ps1 "<테스트 입력>"
```

4. 결과 검증

- 주입 기록:

```powershell
Get-Content -LiteralPath ".repl-json-session\in.applied.log" -Tail 20
```

- 모델 응답/프롬프트 로그:

```powershell
Get-Content -LiteralPath "llm.log" -Tail 120
```

5. 종료

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File bin/repl_json_session_stop.ps1
```

## 운영 규칙

- 시나리오 테스트에서는 `repl_json_session_*` 스크립트만 사용한다.
- 세션 유지 요청이 있을 때는 단발 파이프 실행(`echo ... | bin/infractl.exe --repl`)을 사용하지 않는다.
- 입력마다 새 REPL 프로세스를 시작하지 않는다. 동일 세션에 `send`로 연속 주입한다.

## 트러블슈팅

### `BROKER_ALIVE=False`인데 `STATUS=running`으로 남아있는 경우

stale meta 상태다. 아래 순서로 정리 후 재시작한다.

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File bin/repl_json_session_stop.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File bin/repl_json_session_start.ps1 -ResetLogs
powershell -NoProfile -ExecutionPolicy Bypass -File bin/repl_json_session_status.ps1
```

### `send`는 성공했는데 응답 확인이 늦는 경우

- `.repl-json-session\in.applied.log`에 입력이 기록되었는지 먼저 확인한다.
- `llm.log`는 flush 지연이 있을 수 있으므로 2~5초 대기 후 다시 tail 한다.

### 한글 입력 깨짐 의심

- `in.applied.log`에 한글이 그대로 기록되는지 확인한다.
- `llm.log`의 해당 `MSG` 블록 입력 값과 함께 교차 검증한다.

## 최소 체크리스트 (시나리오 시작 전)

- 세션 상태가 `STATUS=running`, `BROKER_ALIVE=True`
- `.repl-json-session\in.queue.txt`와 `.repl-json-session\in.applied.log`가 초기화됨
- 최근 `llm.log` 시각이 현재 테스트 구간과 일치하는지 확인
