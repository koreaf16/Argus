# 마이그레이션 테스트 기록 (migration_test1.md)

## 테스트 목표
- 원본: oracle-server (MySQL, dcm DB)
- 대상: sandbox-server (PostgreSQL, dcmm DB)

## 테스트 일지
- 2026-04-30: 테스트 시작
- 1차 시도 실패: `bootstrap failed: resume session "-p": open ...\-p.json: The system cannot find the file specified.` (세션 파일 부재)
- 대응: `-r` 옵션 없이 초기 명령 실행 시도
- 2차 시도: `session_id: 81bd7820-4423-478f-b220-d6bb178c7586` 생성됨.
- 진행 상황: 
    - `oracle-server` (MySQL) 접속 성공.
    - `dcm` DB 내 테이블 구조 및 데이터 현황 파악 완료 (item 테이블 등 24개 테이블).
    - `oracle-server`에 `dcmm` DB 생성을 시도하던 중 타임아웃(5분) 발생.
- 문제점: 대규모 데이터 파악 또는 복잡한 툴 호출 과정에서 타임아웃 발생 가능성.
- 대응: `-r` 옵션을 사용하여 기존 세션을 복구하고 계속 진행.
- 3차 시도 실패: 세션 파일(`81bd7820-4423-478f-b220-d6bb178c7586.json`)이 존재하지 않아 복구 불가. (타임아웃으로 인한 저장 실패 추정)
- 버그 발견: `-r` 옵션 사용 시 뒤에 인자가 없으면 다음 인자(`-p`)를 세션 ID로 오인함.
- 대응: `--auto-approve` 옵션을 추가하여 승인 대기로 인한 타임아웃을 방지하고, 새 세션으로 다시 시작.
- 5차 시도 실패: `--aidebug` 모드에서 `--auto-approve` 플래그가 무시되는 버그 발견. 수동 `y/n` 입력 대기로 인해 중단됨.
- 버그 수정: `cmd/argus/main.go` 내 `runAIDebug` 함수가 `flags.autoOK`를 반영하도록 수정 완료.
- 6차 시도: 수정한 코드로 재빌드 후 다시 테스트 진행. `ask_user` 도구 호출 시 승인 대기 상태로 인해 중단됨.
- 버그 수정: `aidebug` 모드에서 `ask_user` 도구 호출 시에도 `--auto-approve` 플래그가 적용되도록 `cmd/argus/main.go` 수정 완료.
- 7차 시도: 이전 세션을 복구하여 자동 승인 모드로 진행. 계획 승인까지 자동으로 처리됨.
- 진행 상황:
    - `sandbox-server`에 `dcmm` DB가 이미 존재함을 확인.
    - `oracle-server`에서 `dcm_dump.sql`을 임포트하거나 DB에 접속하려 했으나 `Access denied` 발생.
- 문제점: `oracle` 사용자 및 `root` 사용자의 MySQL 비밀번호를 알 수 없어 작업 중단. 또한 Argus가 비밀번호를 모를 때 사용자에게 질문하지 않고 다른 우회로(덤프 변환)를 찾으려 함.
- 사용자 힌트: MySQL 비밀번호는 `Gnttkak1!` 임. "모르면 물어보라"는 피드백 수렴.
- 9차 시도: 제공된 비밀번호를 사용하여 마이그레이션 재개 지시. Argus가 기존의 '덤프 파일 sed 변환' 계획을 고수함.
- 진행 상황: `sed` 명령 실행 중 `unbalanced quote` 오류 발생으로 실패.
- 문제점: `sed`를 이용한 SQL 문법 변환은 복잡하고 오류 가능성이 높음. 비밀번호가 있으므로 직접 연결 마이그레이션이 더 효율적임.
- 11차 시도: `sudo -u postgres` 권한으로 `pgloader` 실행 지시.
- 진행 상황: 
    - Bash에서 비밀번호 내 `!` 문자로 인해 `event not found` 오류 발생.
    - 탈출 문자(`\!`) 사용 시에도 `pgloader`가 PostgreSQL 연결 시 `NIL is not of type STRING` 오류 발생.
- 문제점: 쉘 특수문자 처리 미흡 및 `pgloader`의 PostgreSQL TCP 연결 시 인증 요구.
- 12차 시도: 홑따옴표와 유닉스 소켓을 이용한 `pgloader` 재시도.
- 진행 상황: PostgreSQL 연결은 성공했으나, MySQL 8.0 서버와의 프로토콜 호환성 문제(`UNEXPECTED-SEQUENCE-ID`)로 인해 MySQL 접속 실패.
- 문제점: `pgloader`의 MySQL 클라이언트가 MySQL 8.0의 인증 방식 또는 프로토콜을 완벽히 지원하지 못함.
- 13차 시도: 덤프 파일 기반 마이그레이션 재개.
- 진행 상황: 
    - `oracle-server` -> `sandbox-server` 파일 복사 성공.
    - SQL 문법 변환(`sed`) 및 `psql` 임포트 성공.
    - `authority` (2 rows), `chatting`, `item` 등 주요 테이블 마이그레이션 확인 완료.
- 결과: **마이그레이션 성공**.

## 발견 및 수정된 버그
1. **Argus CLI --aidebug 모드 자동 승인 버그**: `--aidebug` 사용 시 `--auto-approve` 플래그를 무시하고 수동 입력을 대기하던 문제 수정 (`cmd/argus/main.go`).
2. **Argus CLI -r 옵션 인자 처리 오류**: `-r` 뒤에 세션 ID가 없으면 다음 옵션을 ID로 오인하는 경향 확인 (대응 완료).
3. **pgloader MySQL 8.0 호환성 문제**: 직접 연결 시 프로토콜 오류 발생 확인 (덤프 기반 방식으로 우회 성공).
4. **쉘 특수문자 처리**: 비밀번호 내 `!` 문자가 Bash 히스토리 확장과 충돌하는 문제 확인 (홑따옴표 사용으로 해결).

## 최종 결론
Argus를 이용한 이기종 DB 마이그레이션 테스트가 성공적으로 마무리되었습니다. 자동 승인 버그 수정을 통해 완전 자동화된 테스트 환경 구축이 가능해졌습니다.

