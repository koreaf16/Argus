# 권한 시스템 (Permission System)

## 1. 권한 체크 흐름

```
Tool Call
  │
  ├── findPreApprovedRule()
  │     ├── Disk 규칙 (~/.argus/permissions/) — 2초 TTL 캐시
  │     └── Session 규칙 (AppState.SessionPermissionRules)
  │     │
  │     ├── 매칭 성공 → 자동 Allow
  │     └── 매칭 실패 → Tool.CheckPermission()
  │           ├── Allow → 직접 실행
  │           ├── Deny → "permission denied"
  │           └── Passthrough
  │                 ├── ReadOnly? → Allow
  │                 └── BehaviorAsk → ApprovalGate.Prompt
  │                       ├── Allow → 실행
  │                       └── Deny → "permission denied"
  │
  └── Tool.Call() → 결과 반환
```

## 2. Permission Rule 매칭

- `toolName` 정확 매칭 (case-insensitive)
- shell 도구: `command` 패턴 매칭
- 빈 `RuleContent` = 전체 허용

## 3. 권한 규칙 출처

| 우선순위 | 출처 | 설명 |
|---------|------|------|
| 1 | Disk | `~/.argus/permissions/` — `LoadAllPermissionRulesFromDisk()` |
| 2 | Session | `AppState` — 런타임 동적 추가 |
| 3 | 캐싱 | 2초 TTL (`permissionRuleCacheTTL`) |

## 4. PermissionResult (types)

```go
type PermissionResult struct {
    Behavior PermissionBehavior
}

type PermissionBehavior string

const (
    BehaviorAllow   PermissionBehavior = "allow"
    BehaviorDeny    PermissionBehavior = "deny"
    BehaviorAsk     PermissionBehavior = "ask"
)
```

## 5. Classification (`internal/utils/permissions/`)

| 패키지 | 설명 |
|--------|------|
| `bash_classifier.go` | Bash 명령어 분류 |
| `dangerous_patterns.go` | 위험 패턴 감지 |
| `yolo_classifier.go` | YOLO 모드 분류 |
| `permission_mode.go` | 권한 모드 (ask/allow/deny) |
| `shell_rule_matching.go` | 셸 규칙 매칭 |
| `shadowed_rule_detection.go` | 가려진 규칙 감지 |

## 6. Plan Mode 권한

- Plan 모드 진입 시 이전 `PermissionMode` 저장
- Plan 모드에서는 Read-Only 도구만 허용
- `ExitPlanMode` 시 이전 모드 복원
- 승인된 plan 단계들은 `ask` 권한을 자동 허용하지만 `deny`는 존중
