# internal/utils — 유틸리티

## bash/ — Bash 파서 (Tree-sitter 기반)

| 파일 | 역할 |
|------|------|
| `bash_parser.go` | Tree-sitter 기반 파서 |
| `commands.go` | 명령어 목록 |
| `heredoc.go` | Heredoc 처리 |
| `interactive_monitor.go` | 대화형 모니터링 |
| `parsed_command.go` | 파싱된 명령 구조체 |
| `pipe_command.go` | 파이프 명령 처리 |
| `prefix.go` / `shell_prefix.go` | 접두사 분석 |
| `registry.go` | 명령 레지스트리 |
| `shell_completion.go` | 셸 완성 |
| `shell_quote.go` / `shell_quoting.go` | 인용부호 처리 |
| `shell_snapshot.go` | 셸 스냅샷 |
| `tree_sitter_analysis.go` | Tree-sitter 분석 |

## permissions/ — 권한 분류기

| 파일 | 역할 |
|------|------|
| `auto_mode_state.go` | 자동 모드 상태 |
| `bash_classifier.go` | Bash 명령어 분류 |
| `bypass_killswitch.go` | 우회 킬스위치 |
| `classifier_decision.go` | 분류 결정 |
| `classifier_shared.go` | 공통 분류 로직 |
| `dangerous_patterns.go` | 위험 패턴 감지 |
| `denial_tracking.go` | 거부 추적 |
| `filesystem.go` | 파일시스템 권한 |
| `get_next_mode.go` | 다음 모드 결정 |
| `path_validation.go` | 경로 검증 |
| `permission_mode.go` | 권한 모드 (ask/allow/deny) |
| `permission_prompt_schema.go` | 승인 프롬프트 스키마 |
| `permission_result.go` | 결과 구조체 |
| `permission_rule_parser.go` | 규칙 파서 |
| `permission_setup.go` | 설정 초기화 |
| `permission_update_schema.go` | 갱신 스키마 |
| `shadowed_rule_detection.go` | 가려진 규칙 감지 |
| `shell_rule_matching.go` | 셸 규칙 매칭 |
| `yolo_classifier.go` | YOLO 모드 분류 |

## powershell/ — PowerShell 분석

| 파일 | 역할 |
|------|------|
| `dangerous_cmdlets.go` | 위험 Cmdlet 목록 |
| `parser.go` | PowerShell 파서 |
| `static_prefix.go` | 정적 접두사 |

## sandbox/ — 샌드박스

| 파일 | 역할 |
|------|------|
| `sandbox_adapter.go` | 샌드박스 어댑터 |
| `sandbox_ui_utils.go` | UI 유틸리티 |

## shell/ — 셸 유틸리티

| 파일 | 역할 |
|------|------|
| `read_only_command_validation.go` | Read-Only 검증 |
| `read_only_gh_commands.go` | GitHub read-only 명령 |
| `read_only_git_commands.go` | Git read-only 명령 |
| `resolve_default_shell.go` | 기본 셸 해결 |
| `shell_provider.go` | 셸 공급자 |
| `shell_tool_utils.go` | Tool 유틸리티 |
| `spec_prefix.go` | Spec 접두사 |

## 최상위 유틸리티

| 파일 | 역할 |
|------|------|
| `collapse_background_bash.go` | 백그라운드 Bash 접기 |
| `prompt_shell_execution.go` | 셸 실행 프롬프트 |
| `pty_support_unix.go` / `pty_support_windows.go` | PTY 지원 (플랫폼별) |
| `shell.go` / `shell_command.go` / `shell_command.go` | 셸 헬퍼 |
| `utils.go` | 공통 유틸리티 |
