# 세션 관리 (Session Management)

## 1. 세션 ID 생성

[`internal/session/session.go`](internal/session/session.go:40) 는 UUIDv4 호환 세션 ID를 생성합니다.

```go
func NewID() (string, error) {
    var b [16]byte
    rand.Read(b[:])
    b[6] = (b[6] & 0x0f) | 0x40  // version 4
    b[8] = (b[8] & 0x3f) | 0x80  // variant
    // "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx"
}
```

## 2. Snapshot 구조

### v1 (기존)
```json
{
    "saved_at": "2024-01-01T00:00:00Z",
    "messages": [...]
}
```

### v2 (신규)
```json
{
    "version": 2,
    "saved_at": "2024-01-01T00:00:00Z",
    "messages": [...],
    "graph": [...],
    "artifacts": [...]
}
```

| 필드 | v1 | v2 | 설명 |
|------|----|----|------|
| `version` | X | 2 | 버전 식별 (0/1 = v1, 2 = v2) |
| `saved_at` | ✓ | ✓ | 저장 시간 |
| `messages` | ✓ | ✓ | 메시지 배열 (v1 호환) |
| `graph` | X | ✓ | Context Graph 노드 |
| `artifacts` | X | ✓ | 아티팩트 참조 |

## 3. 저장 (persistSessionSnapshot)

```
Turn 종료 → StopHook 발화
  │
  ├── SanitizeMessagesForStorage()
  │
  ├── Snapshot 생성
  │     ├── Version: 2
  │     ├── Messages: e.messages
  │     ├── Graph: graph.Nodes()
  │     └── Artifacts: artMF.Refs()
  │
  └── memStore.SaveSession()
        └── NDJSON 파일 저장
              (~/.argus/sessions/{session_id}.ndjson)
```

## 4. 복원 (--resume / -r)

```
initializeSession(sessionID)
  │
  ├── memStore.LoadSession(sessionID)
  │     └── NDJSON 파일 로드
  │
  ├── Snapshot 역직렬화
  │     ├── v1: Messages[]만
  │     └── v2: Messages[] + Graph[] + Artifacts[]
  │
  ├── engine.ReplaceMessages()
  │     └── graph 재구축, 노드 시퀀스 복원
  │
  └── 다음 Turn 계속
```

## 5. CLI 플래그

| 플래그 | 설명 |
|--------|------|
| `--resume <id>` / `-r <id>` | 세션 ID로 대화 복원 |

세션 종료 시 `session_id: <id>`가 stderr에 출력됩니다.

## 6. Per-turn Auto-save

각 Turn 종료 시 Engine StopHook을 통해 자동 저장됩니다:

1. Turn 완료 → `StopHook` 발화
2. `persistSessionSnapshot()` 호출
3. NDJSON 원자적 저장 (temp file + rename)
