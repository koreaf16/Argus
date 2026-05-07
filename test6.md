# 서버 시스템 인벤토리 + LLM Prompt 의미론적 주입 + UI 빨간색 수정 — 작업 계획서 (v2)

> v1 → v2 변경: Codex 분석을 받아 PR1 범위를 축소하고, 코드 검증으로 사실관계를 정정. 핵심 변화는 ① 빨간색 원인 확정(영어/한국어 mismatch), ② Oracle/WAS/MQ는 PR2로 이동, ③ Inventory 전용 SSH purpose channel, ④ K8s pod hosting 매핑 전략 변경, ⑤ ConfigDir() 경로 사용, ⑥ singleflight + epoch 검증.

---

## Context (정정)

현재 Argus의 서버 흐름에서 다음 문제가 있습니다.

1. **의미론적 인벤토리 부재** — [internal/query/workspace_prompt.go:137-173](internal/query/workspace_prompt.go) 가 이미 OS/shell/user/cwd/uptime/memory/listeners/services/docker 평문을 system prompt에 주입하고 있습니다 (v1 표현 정정: "주입되지 않는다" → "주입은 되지만 의미론적 분류가 없다"). 즉 "vLLM의 gemma4가 어느 pod/container/native에 떠있는지", "OracleDB가 컨테이너DB인지", "Tomcat 버전이 무엇인지" 같은 **분류·매핑 정보가 없습니다**. 사용자가 "vLLM의 gemma4 확인해줘"라고 해도 LLM은 hosting 형태를 추론해야 합니다.

2. **UI 빨간색 트리 — 원인 확정** — `server_connect` 도구가 [serverconnect.go:187](internal/tools/serverconnect/serverconnect.go) 에서 한국어 `"%s에 연결되었습니다. 환경 정보를 확인 중입니다..."` 를 emit하는데, 렌더러는 [ui.go:185](internal/tools/serverconnect/ui.go) 에서 영어 `strings.Contains(resultText, "Connected to")` 만 매칭합니다. 한국어 메시지가 매칭되지 않아 "에러 결과" 분기로 빠지고 `FormatResultLines(..., isError=true, ...)` 경로로 본문 전체가 `StatusErrorColor`로 칠해집니다. 트리 라인 `└` 자체가 아니라 본문이 빨간색입니다.

3. **두 경로 UI 불일치** — `/server` 모달이 `presentation.EventSystem` 평문 라인을 직접 emit하여 도구 트리 UI를 우회. (PR2로 분리)

사용자 결정: **PR1 = 축소된 인벤토리 + LLM 주입 + UI 빨간색 즉시 수정**, **PR2 = Oracle/WAS/MQ + 모달 통일**.

---

## Codex 분석 반영표

| Codex 지적 | 우리 대응 |
|---|---|
| PR1 범위 과대 | **Oracle/WAS/MQ 전부 PR2로 이동**. PR1 = LLMServing + Docker + K8s pod/container ID 매핑 + cache + prompt + `/server scan` + UI 빨간색 |
| "system prompt 미주입" 표현 과장 | "OS/서비스 평문은 이미 주입됨. 의미론적 분류(모델, CDB/PDB, hosting 매핑)가 없음" 으로 정정 |
| 빨간색은 문자열 heuristic 취약 | **typed status event** 도입. `tool.NewStatusEvent("server_connect.connected")` 신규 또는 marker prefix `[ARGUS_SERVER_CONNECT:connected]\n` 사용. heuristic 제거 |
| RenderToolResult가 정상 결과를 통째로 숨김 | interactive 활성 경로에서만 숨김. headless/replay/non-stream 경로는 정상 렌더. `theme.IsInteractive()` 또는 `RenderContext` 플래그 추가 |
| InvokeProgrammatic 설계가 코드와 mismatch | 레지스트리 메서드는 `Lookup(name)` ([registry.go:122](internal/tools/registry.go)). TUI Config에는 tool registry 없고 LLM model registry만. **PR2에서 `Engine.InvokeToolForUI()` 공개 메서드 신설** 또는 TUI config에 tool registry 명시적 주입. **PR1에서는 modal 통일 보류** |
| ExecWithOptions가 사용자 lane 점유 | **`channel.PurposeInventory` 신규 추가**. [metrics_channel.go:83-86](internal/services/workspace/channel/metrics_channel.go) + [manager.go:116](internal/services/workspace/channel/manager.go) 패턴 확장 → 별도 SSH session으로 inventory 수집 |
| Marker 순서 위험 | inspect.go:92-99 패턴 그대로 따름: `printf '<<ARGUS_PROBE:%s>>\n'` 먼저, 그 후 명령. 첫 섹션은 첫 marker 이전이라 자연스레 비어있음 |
| `/v1/models`는 K8s false negative | **K8s는 `kubectl get pods -A -o json` 의 container args/env/ports + `status.containerStatuses[].containerID` 매핑이 기본**. `/v1/models`는 native/docker에만. `kubectl exec curl /v1/models`는 **`--deep` 명시 옵션** (PR3+) |
| Cache disconnect 정책 | disconnect 시 **inflight scan 취소 + memory cache만 정리**. disk cache는 force scan / 설정 변경 / TTL 만료 시만 무효화 |
| `~/.argus` 경로 오류 | **`constants.ConfigDir()` 사용** ([paths.go:30](internal/constants/paths.go)). 신규 헬퍼: `constants.InventoryCacheDir()` = `<ConfigDir>/inventory-cache/` |
| async callback에 다른 서버 인벤토리 섞임 | `Snapshot.Alias` + `epoch uint64` (alias마다 증가) 검증. 늦게 도착한 callback은 drop |
| 동일 alias 반복 connect 시 중복 scan | `golang.org/x/sync/singleflight` per-alias key. 진행 중인 scan 공유 |
| alias filename 충돌 | `sha256(alias+host+user)[0:16]` hex + safe slug. 특수문자/대소문자 안전 |
| `ss -tlnp` PID 숨김 | sudo 없이 PID 안 보이면 `lsof -nP -iTCP -sTCP:LISTEN` fallback. 그것도 실패 시 `Hosting.Type="unknown"` 명시 (감추지 않음) |
| cgroup parser 다양성 | testdata fixture: `cgroup_v1_docker.txt`, `cgroup_v2_systemd.txt`, `cgroup_kubepods.txt`, `cgroup_containerd.txt`, `cgroup_rootless.txt` |
| curl 순차 10초+ | ss로 열린 포트만 좁힘. bash `& wait` 또는 `xargs -P 4` 병렬 |
| Oracle sysdba 자동 실행 위험 | **PR1에서 oracle probe 완전 제외**. PR2에 `--subset=oracle --deep` 명시 옵션만 |

---

## PR1 — 축소판 (Codex 권장)

### 1.1 패키지 구조

```
internal/services/inventory/
├── runner.go            오케스트레이션 + singleflight (≤220 줄)
├── types.go             InventorySnapshot 등 (≤180 줄)
├── exec.go              ProbeExec 어댑터 (PurposeInventory channel 사용) (≤100 줄)
├── summary.go           FormatSummaryForPrompt + 토큰 예산 truncation (≤250 줄)
├── cache.go             메모리(5분) + 디스크(15분) 2단 캐시 + sha256 키 (≤200 줄)
├── budget.go            우선순위 truncation (≤80 줄)
├── runner_test.go
├── summary_test.go
├── cache_test.go
└── probes/
    ├── probe.go         Probe 인터페이스, Result sum-type (≤60 줄)
    ├── docker.go        (≤200 줄)
    ├── kubernetes.go    (≤280 줄, kubectl get -o json + containerID 매핑)
    ├── llm_serving.go   (≤220 줄, /v1/models for native/docker only)
    ├── *_test.go + testdata/
```

PR1에 포함된 probe = **3개** (Docker, Kubernetes, LLM Serving). Oracle/WAS/MQ는 PR2.

### 1.2 데이터 모델 (확정 필드)

```go
type InventorySnapshot struct {
    Alias       string
    Epoch       uint64           // 늦게 도착한 callback 검증용
    CollectedAt time.Time
    DurationMs  int64
    Status      InventoryStatus  // pending|ready|partial|failed

    Containers  []ContainerInfo
    Kubernetes  *K8sInfo
    LLMServing  []LLMServingInfo

    Errors      map[string]string // probe별 actionable 메시지
    ArtifactID  string
}

type LLMServingInfo struct {
    Engine   string        // "vllm" | "ollama" | "triton" | "tgi" | "sglang"
    Models   []LLMModel
    Port     int
    Endpoint string
    Hosting  LLMHosting
}
type LLMHosting struct {
    Type      string  // "k8s" | "docker" | "native" | "unknown"
    K8sPod    string  // "vllm-system/vllm-gemma" (k8s only)
    K8sContainer string // pod 내 container 이름
    Container string  // docker name
    Pid       int
    Cmdline   string  // 첫 80자
}
```

### 1.3 SSH Inventory Channel 신설

**기존 코드 확장**:
- [internal/services/workspace/channel/types.go](internal/services/workspace/channel/types.go) 또는 동급 파일에 `PurposeInventory` 상수 추가
- 신규 파일 `internal/services/workspace/channel/inventory_channel.go` — `metrics_channel.go` 그대로 모방

```go
// inventory_channel.go (metrics_channel.go 86줄 패턴 그대로 모방)
type inventoryChannel struct {
    key      ChannelKey
    client   *ssh.Client
    state    State
    closed   atomic.Bool
    mu       sync.Mutex
    lastUsed time.Time
}

// 핵심: 사용자 exec lane을 점유하지 않고 fresh ssh.Session에서 실행
func (c *inventoryChannel) Run(ctx context.Context, script string) (string, error) {
    if c.closed.Load() {
        return "", fmt.Errorf("channel: inventory closed")
    }
    session, err := c.client.NewSession()
    if err != nil { return "", fmt.Errorf("inventory: open session: %w", err) }
    defer session.Close()
    var out, errBuf bytes.Buffer
    session.Stdout = &out
    session.Stderr = &errBuf
    done := make(chan error, 1)
    go func() { done <- session.Run(script) }()
    select {
    case <-ctx.Done():
        _ = session.Signal(ssh.SIGTERM)
        return out.String(), ctx.Err()
    case err := <-done:
        return out.String(), err
    }
}
```

[manager.go:116](internal/services/workspace/channel/manager.go) 패턴으로 `AcquireInventory(ctx, alias)` 메서드 추가. `internal/services/inventory/exec.go` 의 `ProbeExec` 가 `Manager.AcquireInventory(...).Run(...)` 호출.

이렇게 하면 **사용자가 bash 명령 실행 중이어도 inventory 수집이 lane을 막지 않음**.

### 1.4 Probe 인터페이스 + 단일 batched 스크립트

```go
// probes/probe.go
type Probe interface {
    Name() string
    PreferredTimeout() time.Duration
    ScriptFragment() string                  // bash, set +e
    Parse(stdout string) (Result, error)
}
```

`runner.go`가 Probe들의 `ScriptFragment()`를 **inspect.go 패턴 그대로** 합쳐 1회 실행:

```bash
# inspect.go:92-99 패턴: marker-first
printf '<<ARGUS_PROBE:docker>>\n'
{ command -v docker >/dev/null 2>&1 && docker ps -a --format '{{json .}}' 2>/dev/null; } 2>/dev/null
printf '<<ARGUS_PROBE:kubernetes>>\n'
{ kubectl config current-context; kubectl get pods -A -o json 2>/dev/null | head -c 65536; ... } 2>/dev/null
printf '<<ARGUS_PROBE:llm_serving>>\n'
{ ... }
```

Split 시 첫 섹션(첫 marker 이전)은 비어있음 → 모든 probe 결과가 안전하게 잡힘.

### 1.5 Kubernetes probe 전략 (Codex 권장 따름)

`/v1/models` 는 host의 localhost에서 pod 내부 vLLM에 닿지 않는 경우가 많음. 따라서:

```bash
# 1단계: kubectl 가용성 + 권한
ls -la "${KUBECONFIG:-$HOME/.kube/config}" 2>/dev/null
kubectl config current-context 2>/dev/null
kubectl auth can-i list pods --all-namespaces 2>/dev/null

# 2단계: 정형 JSON 1회로 노드/네임스페이스/파드 모두
kubectl get nodes -o json 2>/dev/null | head -c 32768
kubectl get pods -A -o json 2>/dev/null | head -c 131072
kubectl get svc -A -o json 2>/dev/null | head -c 32768
```

`Parse()` 로직:
- `pods[].spec.containers[].image`, `pods[].spec.containers[].args`, `pods[].spec.containers[].env`, `pods[].spec.containers[].ports[].containerPort`
- `status.containerStatuses[].containerID` (예: `containerd://abc123...` 또는 `docker://abc123...`) — **이게 LLM Serving probe의 cgroup 매핑과 cross-reference 키**
- 이미지가 vllm/sglang/tgi/triton 류면 LLM Serving 후보로 추가하고 `Hosting.Type="k8s"`, `K8sPod="<ns>/<name>"`, `K8sContainer="<container>"` 채움
- 모델명은 args에서 `--model` 또는 `--served-model-name` 파싱. env에서 `MODEL_PATH` 등 보조

### 1.6 LLM Serving probe (native/docker 우선)

```bash
# 1단계: 후보 프로세스 + 후보 포트 + cgroup
pgrep -fa 'vllm\|python.*-m vllm\|sglang\|tgi\|text-generation\|ollama' 2>/dev/null
echo '<<SECTION_PORTS>>'
ss -tlnp 2>/dev/null | grep -E ':(8000|8001|8002|8003|11434|8080|8081|8082)' | head -20
# fallback: ss -tlnp가 PID 숨길 시
lsof -nP -iTCP -sTCP:LISTEN 2>/dev/null | head -40
echo '<<SECTION_CGROUP>>'
for pid in $(pgrep -f 'vllm\|sglang\|tgi\|ollama' 2>/dev/null); do
    echo "pid=$pid cmd=$(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ' | head -c 200)"
    cat /proc/$pid/cgroup 2>/dev/null
    echo '---'
done
echo '<<SECTION_OLLAMA>>'
command -v ollama >/dev/null 2>&1 && ollama list 2>/dev/null
echo '<<SECTION_MODELS>>'
# /v1/models는 native/docker만. K8s pod는 host에서 안 닿음.
# bash 병렬: & wait
for port in 8000 8001 8002 8003 11434 8080 8081; do
    (curl -s -m 2 http://localhost:$port/v1/models 2>/dev/null && echo "===PORT_${port}_END===") &
done
wait
```

`Parse()` 로직:
1. cgroup에 `kubepods` 포함 → 해당 PID는 K8s probe의 매핑에 위임 (이 probe에서는 `Hosting.Type="k8s"` 만 표시하고 자세한 pod 정보는 K8s probe 결과에서 가져와 합침. runner의 `merge` 단계에서 cross-reference)
2. cgroup에 `docker[/-]([a-f0-9]{12})` 매칭 → docker probe 결과의 ID와 매칭하여 Container 이름 채움. `Hosting.Type="docker"`
3. 둘 다 없으면 `Hosting.Type="native"`
4. `MODELS` 섹션의 OpenAI-호환 JSON 응답에서 `data[].id` 추출 → `LLMModel.ID`. 응답 없으면 cmdline에서 `--model` 또는 `--served-model-name` 파싱 fallback

### 1.7 Cache (코덱스 권장 정책)

```go
// cache.go
type cacheKey string
func makeCacheKey(alias, host, user string) cacheKey {
    h := sha256.Sum256([]byte(alias + "|" + host + "|" + user))
    return cacheKey(hex.EncodeToString(h[:8])) + "_" + safeSlug(alias)
    // 예: "a1b2c3d4e5f6a7b8_a100-server"
}

type Cache struct {
    mu       sync.RWMutex
    mem      map[string]cachedEntry
    diskDir  string                  // constants.InventoryCacheDir()
    memTTL   time.Duration           // 5분
    diskTTL  time.Duration           // 15분
}

func (c *Cache) Get(key cacheKey) (Snapshot, bool) {
    // 1) memory hit (5분 이내) → 반환
    // 2) disk hit (15분 이내) → memory에 promote 후 반환
    // 3) miss
}

func (c *Cache) Set(key cacheKey, snap Snapshot) {
    // memory + disk 동시 저장
}

func (c *Cache) InvalidateMemory(key cacheKey)  // disconnect 시 호출
func (c *Cache) ForceInvalidate(key cacheKey)   // force scan / 설정 변경 시
```

**disconnect 정책** (Codex 권장):
- inflight scan ctx cancel
- memory cache invalidate
- **disk cache는 보존** → 다음 connect 시 즉시 사용 가능

### 1.8 Singleflight + Epoch 검증

```go
// runner.go
type Runner struct {
    workspace    *workspace.Manager
    cache        *Cache
    probes       []probes.Probe
    sf           singleflight.Group
    epochs       sync.Map          // alias → uint64
}

func (r *Runner) CollectAsync(ctx context.Context, alias string,
    onDone func(Snapshot)) {
    // alias별 epoch 증가
    epoch := r.bumpEpoch(alias)

    go func() {
        // singleflight: 동일 alias 중복 호출 시 한 번만 실행
        result, _, _ := r.sf.Do(alias, func() (interface{}, error) {
            return r.collect(ctx, alias, epoch), nil
        })
        snap := result.(Snapshot)

        // epoch 검증: 더 새로운 connect가 있었으면 drop
        if currentEpoch, _ := r.epochs.Load(alias); currentEpoch != epoch {
            return  // drop
        }
        r.cache.Set(makeCacheKey(...), snap)
        if onDone != nil { onDone(snap) }
    }()
}

func (r *Runner) bumpEpoch(alias string) uint64 {
    val, _ := r.epochs.LoadOrStore(alias, uint64(0))
    next := val.(uint64) + 1
    r.epochs.Store(alias, next)
    return next
}
```

### 1.9 LLM Prompt 주입

`internal/query/inventory_prompt.go`:

```go
const InventoryTokenBudget = 1500

func inventorySystemBlocks(manager *workspace.Manager) []llm.SystemBlock {
    if manager == nil { return nil }
    if !inventoryEnabled() { return nil }
    active := manager.ActiveAlias()
    if active == workspace.LocalAlias { return nil }

    snap, ok := manager.GetInventorySnapshot(active)
    if !ok {
        return placeholderBlock("의미론적 인벤토리: 아직 수집되지 않음. server_inventory 도구로 수집 가능.")
    }
    if snap.Status == inventory.StatusPending {
        return placeholderBlock("의미론적 인벤토리: 백그라운드 수집 진행 중. 다음 턴에서 자동 갱신.")
    }
    return []llm.SystemBlock{{
        Type:         "text",
        Text:         inventory.FormatSummaryForPrompt(snap, InventoryTokenBudget),
        CacheControl: map[string]any{"type": "ephemeral"},
    }}
}
```

조립 위치 — [internal/query/engine_run.go](internal/query/engine_run.go) ~288:

```go
sysBlocks := JoinSystemBlocks(
    systemFn(),
    workspaceSystemBlocks(deps.Workspace),  // 기존 OS/services 평문 (그대로 유지)
    inventorySystemBlocks(deps.Workspace),  // 신규 의미론적 분류
    laneSystemBlocks(deps.Workspace),
)
```

**중요**: workspace_prompt.go의 기존 inspect 평문 주입은 **그대로 둠** (PR1에서 제거 안 함). 인벤토리는 그 위에 추가 정보를 얹는 형태. 토큰 중복은 PR3+에서 정리.

### 1.10 토큰 예산 + 우선순위 (PR1 카테고리 한정)

PR1 카테고리만:
1. **LLM Serving** (사용자 핵심 질의)
2. **Kubernetes** (네임스페이스 + 주요 워크로드)
3. **Containers** (Docker)

예산 1500 토큰 초과 시:
- LLM Serving: 항상 풀로
- Kubernetes pods: head 10 → 5 → 3 → "(전체 N개)"
- Containers: head 10 → 5 → 3 → "(전체 N개)"

### 1.11 LLM 주입 텍스트 예시 (PR1)

```
의미론적 서버 인벤토리 (alias=a100-server, 수집=2026-05-07 14:32 KST, 4.8s, status=ready):

[LLM Serving]  ← 사용자 모델 질의 라우팅용
  - vllm   → google/gemma-2-27b-it     @ :8000  hosting=k8s pod (vllm-system/vllm-gemma, container=vllm)
  - vllm   → meta-llama/Llama-3.1-70B  @ :8001  hosting=docker (container=vllm-llama, image=vllm/vllm-openai:0.5.4)
  - ollama → llama3:8b, mistral:7b              hosting=native (pid=4567)

[Kubernetes] context=ai-cluster, perm=admin, 3 nodes / 12 pods / 4 ns
  네임스페이스: default, kube-system, vllm-system, monitoring
  주요 워크로드:
    - vllm-system/vllm-gemma  (Running 1/1, image=vllm/vllm-openai:0.5.4, container vllm:8000)
    - vllm-system/vllm-qwen   (Running 1/1)
    - monitoring/grafana      (Running 1/1)

[Containers] docker: 5개 (running 3 / exited 2)
  - vllm-llama (vllm/vllm-openai:0.5.4) 8001->8000/tcp [running]
  - postgres-dev (postgres:15) 5432/tcp [running]
  - cache-redis (redis:7-alpine) 6379/tcp [running]

주의: 이 정보는 5분간 유효. server_inventory 재호출 또는 /server scan 으로 갱신.
Oracle/WAS/MQ 등 추가 카테고리는 PR2 머지 후 사용 가능.
```

### 1.12 신규 도구 + 슬래시 명령

**도구 `server_inventory`** — `internal/tools/serverinventory/serverinventory.go`:
- Name: `"server_inventory"` (snake_case 검증: `Name() string` 반환값)
- Input: `{"server": string, "force": bool}`
- 등록: [cmd/argus/main.go](cmd/argus/main.go) tool registry
- safeAutoModeTools: [classifier_decision.go:8-27](internal/utils/permissions/classifier_decision.go) → `"server_inventory": true`
- canonical alias: 충돌 없음

**슬래시 `/server scan [alias]`** — [dispatcher.go](internal/repl/commands/dispatcher.go) `handleServer` switch에 `case "scan":` 추가. `--subset` 옵션은 PR2에서 추가 (현재 PR1은 카테고리 3개뿐이라 subset 선택지 적음).

### 1.13 UI 빨간색 수정 (PR1 포함)

**원인 확정**: server_connect 한국어 emit ↔ renderer 영어 매칭.

**해결안 — typed status event** (Codex 권장의 "구조화된 상태"):

옵션 A (가벼움): tool emit에 marker prefix 사용
```go
// internal/tools/serverconnect/serverconnect.go:187
events <- tool.NewOutputEvent(fmt.Sprintf(
    "[ARGUS_SERVER_CONNECT:connected]\n%s에 연결되었습니다. 환경 정보를 확인 중입니다...\n",
    resolvedAlias))
```
```go
// internal/tools/serverconnect/ui.go:185
if strings.HasPrefix(resultText, "[ARGUS_SERVER_CONNECT:connected]") ||
   strings.Contains(resultText, "[ARGUS_SERVER_CONNECT:connected]") {
    return ""  // interactive가 이미 표시함
}
```

옵션 B (구조적): `tool.ToolEvent` 에 `EventStatus` 종류 신설
```go
// internal/services/tools/event.go (또는 동급)
type EventKind int
const (
    EventOutput EventKind = iota
    EventError
    EventDone
    EventStatus // 신규
)
type ToolEvent struct {
    Kind   EventKind
    Text   string
    Error  error
    Status string // "connected", "scanning", "ready"
}
```

**PR1 채택**: 옵션 A (작은 변경, 즉시 효과). 옵션 B는 PR3+에서 type cleanup 시 도입.

**RenderToolResult 안전성** — Codex 지적 "headless/replay 경로에서 정상 결과 사라짐":

```go
func (r *ServerConnectRenderer) RenderToolResult(resultText string, _ int64, theme toolui.ThemeContext) string {
    resultText = strings.TrimSpace(resultText)
    if resultText == "" { return "" }

    isConnected := strings.Contains(resultText, "[ARGUS_SERVER_CONNECT:connected]")

    // interactive가 활성이면 성공 결과는 InteractiveModel이 이미 그림 → 숨김
    // interactive 비활성(headless/replay/non-stream) → 정상 렌더
    if isConnected && theme.IsInteractive() {
        return ""
    }
    if isConnected {
        // 마커 제거 후 본문만 정상 색으로 렌더
        body := strings.TrimSpace(strings.Replace(resultText,
            "[ARGUS_SERVER_CONNECT:connected]", "", 1))
        var lines []string
        for _, l := range strings.Split(body, "\n") {
            if t := strings.TrimSpace(l); t != "" { lines = append(lines, t) }
        }
        return toolui.FormatResultLines(lines, true, false, theme)  // isError=false
    }

    // 마커 없으면 명시적 실패 패턴 검사
    lower := strings.ToLower(resultText)
    isError := strings.HasPrefix(lower, "error") ||
               strings.HasPrefix(lower, "failed") ||
               (strings.Contains(lower, "connect") && strings.Contains(lower, "fail"))
    var lines []string
    for _, l := range strings.Split(resultText, "\n") {
        if t := strings.TrimSpace(l); t != "" { lines = append(lines, t) }
    }
    return toolui.FormatResultLines(lines, true, isError, theme)
}
```

`theme.IsInteractive()` 신규 메서드 — `ThemeContext` 인터페이스에 추가. 기본 구현은 `!theme.DisableANSI && !aiDebugMode` 같은 식.

---

## PR2 — Oracle/WAS/MQ + 모달 통일 (후속)

PR1 머지 후:

### 2.1 추가 Probes
- `probes/oracle.go` — sysdba는 `--deep` 옵션 명시 시만. 기본은 `lsnrctl status` + `ps -ef | grep ora_pmon` + oratab 파싱만
- `probes/was.go` — tomcat/wildfly/weblogic/jeus/nginx/apache
- `probes/mq.go` — kafka/rabbitmq/redis

### 2.2 모달 통일 (`Engine.InvokeToolForUI`)

`internal/query/engine.go` 또는 `engine_run.go`:
```go
// LLM 턴을 거치지 않고 도구를 직접 invoke. UI 모달이 사용.
// 결과는 transcript에 ToolUse/ToolResult 블록으로 emit.
func (e *Engine) InvokeToolForUI(ctx context.Context, name string,
    input json.RawMessage) (<-chan tool.ToolEvent, error) {
    t, ok := e.toolRegistry.Lookup(name)
    if !ok { return nil, fmt.Errorf("tool %q not found", name) }
    toolCtx := e.makeUIToolContext(ctx)
    return t.Call(toolCtx, input)
}
```

[internal/tui/modal_server_list.go:172-210](internal/tui/modal_server_list.go) actionConnect 핸들러를:
- `presentation.EventSystem` 평문 emit 제거
- `m.app.engine.InvokeToolForUI(ctx, "server_connect", input)` 호출
- ToolEvent 채널 → transcript의 toolUse 메시지로 emit

### 2.3 InteractiveModel 인벤토리 페이즈

`ServerConnectRenderer.InteractiveModel` 에 `phaseInventoryScanning`/`phaseInventoryReady` 추가. runner의 `onInventoryReady` 콜백 → 도구 출력 채널로 마커 (`[ARGUS_INVENTORY:ready]\n` prefix) emit. View()가 두 번째 branch 블록 렌더.

### 2.4 `--subset` 옵션
PR2부터 의미 있음. `/server scan --subset=docker,k8s,llm` 또는 `--subset=oracle,was --deep`

---

## PR3+ (장기)

- `kubectl exec curl /v1/models` (`--deep`)
- osquery 자동 감지 시 SQL 1회로 대체
- typed `tool.EventStatus` 도입 (옵션 A → B 마이그레이션)
- workspace_prompt.go의 inspect 평문 주입을 inventory 요약으로 대체 (토큰 dedup)
- multi-channel 추가 최적화

---

## UI 카드 랜더링 예상도

### PR1 완료 시점

`/server connect a100-server` (모달 경로 — UI는 그대로):
```
ServerConnect(a100-server)
  ⎿  ✔ Connected as koreaf16     ← PR1: 회색이 아닌 정상 색 (빨간색 회귀 X)
     OS: Linux aisvr 5.14 / services: 26 / listeners: 17
     a100-server에 연결되었습니다. 환경 정보를 확인 중입니다...
```

다음 LLM 턴 (system prompt에 인벤토리 자동 주입):
```
> vLLM의 gemma 모델 어디에 떠있는지 알려줘
[Assistant] gemma-2-27b 모델은 K8s pod vllm-system/vllm-gemma 의 vllm 컨테이너에서
실행 중입니다 (image: vllm/vllm-openai:0.5.4, port 8000).
```

### PR2 완료 시점

```
ServerConnect(a100-server)
  ⎿  ✔ Connected as koreaf16
     OS: Linux aisvr 5.14 / services: 26 / listeners: 17
  ⎿  🔎 inventory scanning ...
```

~5초 후:
```
ServerConnect(a100-server)
  ⎿  ✔ Connected as koreaf16
     OS: Linux aisvr 5.14 / services: 26 / listeners: 17
  ⎿  ✔ inventory ready (4.8s)
     ▸ llm serving    gemma-2-27b @ :8000  →  k8s (vllm-system/vllm-gemma)
                      Llama-3.1-70B @ :8001  →  docker (vllm-llama)
     ▸ kubernetes     3 nodes / 12 pods / 4 namespaces
     ▸ containers     docker: 5 (running 3)
     ▸ databases      oracle 19c CDB ORCLCDB / PDBs: PDB1, PDB2  ← PR2부터
     ▸ was            tomcat 9.0.85 @ :8080                       ← PR2부터
     ▸ mq             redis 7.2.4 @ :6379                         ← PR2부터
```

색상:
- `⎿` 회색 — `theme.MutedColor()`
- `✔` 녹색 — `theme.StatusSuccessColor()`
- `🔎` 회색
- 카테고리 이름 청색 Bold — `theme.ToolUseTitleColor()`
- 값 본문 흰색 — `theme.BodyColor()`
- 부가 메타 (괄호, `→`) 회색

---

## 위험 요소와 완화 (Codex 분석 통합)

| 위험 | 완화 |
|---|---|
| async callback alias mismatch | `Snapshot.Epoch` + `r.epochs sync.Map`. 콜백 시 현재 epoch와 비교, 불일치면 drop |
| 중복 connect → 중복 scan goroutine | `singleflight.Group` per-alias |
| alias filename 충돌 | `sha256(alias+host+user)[0:16]` hex prefix + safe slug |
| `ss -tlnp` PID 숨김 | `lsof -nP -iTCP -sTCP:LISTEN` fallback. 실패 시 `Hosting.Type="unknown"` 명시 (감추지 않음) |
| cgroup parser 다양성 | testdata fixture 5종 (v1 docker, v2 systemd, kubepods, containerd, rootless) |
| curl 순차 10초+ | bash `& wait` 병렬, ss로 열린 포트만 좁힘. 글로벌 25초 안에 들어옴 |
| Oracle sysdba 자동 실행 위험 | **PR1에서 oracle probe 없음**. PR2에서 `--deep` 옵션만 |
| docker 그룹 미가입 / kubectl 권한 없음 | probe 1단계 가드. `Errors`에 actionable 메시지 |
| pod 1000개, 컨테이너 100개 | head 50, summary 토큰 1500 초과 시 우선순위 truncation |
| 인벤토리 미준비 상태 첫 LLM 질문 | placeholder block + 디스크 캐시로 재시작 후 즉시 사용 |
| SSH lane 점유 | `PurposeInventory` 별도 channel. 사용자 exec lane과 분리 |
| Marker 순서 | inspect.go 패턴 그대로 (marker-first) |
| `~/.argus` vs `cwd/.Argus` | `constants.ConfigDir()` + 신규 `InventoryCacheDir()` |
| `RenderToolResult` 결과 통째 사라짐 | `theme.IsInteractive()` 분기 |
| 빨간색 heuristic 취약 | `[ARGUS_SERVER_CONNECT:connected]` 마커. 이전 영어 contains 매칭 폐기 |

---

## 수정/신규 파일 목록 (PR1 한정)

### 신규
- `internal/services/inventory/runner.go`, `types.go`, `exec.go`, `summary.go`, `cache.go`, `budget.go`
- `internal/services/inventory/probes/probe.go`, `docker.go`, `kubernetes.go`, `llm_serving.go`
- 각 probe별 `*_test.go` + `testdata/`
- `internal/services/workspace/channel/inventory_channel.go` — metrics_channel.go 패턴 모방
- `internal/tools/serverinventory/serverinventory.go`, `ui.go`
- `internal/query/inventory_prompt.go` + `*_test.go`
- `internal/constants/inventory_paths.go` — `InventoryCacheDir()`, `InventoryCachePath(key)`

### 수정
- `internal/services/workspace/channel/types.go` — `PurposeInventory` 상수 추가
- `internal/services/workspace/channel/manager.go` — `AcquireInventory(ctx, alias)` 메서드 추가 (라인 116 패턴)
- `internal/services/workspace/manager.go` — `inventoryRunner` 필드 + `ConnectActivateAndScan` / `GetInventorySnapshot` / `RescanInventory`. NewManager에서 runner 초기화. Disconnect에서 inflight 취소 + memory cache invalidate (디스크는 보존)
- `internal/tools/serverconnect/serverconnect.go:187` — `[ARGUS_SERVER_CONNECT:connected]\n` 마커 prefix 추가 + ConnectActivateAndScan 사용
- `internal/tools/serverconnect/ui.go:179-196` — 마커 기반 분기. `theme.IsInteractive()` 체크. 한국어/영어 contains 매칭 제거
- `internal/tui/toolui/renderer.go` — `ThemeContext` 인터페이스에 `IsInteractive() bool` 추가
- `internal/tui/theme.go` — uiTheme에 IsInteractive 구현
- `internal/repl/commands/dispatcher.go` `handleServer` — `case "connect":` 갱신, `case "scan":` 추가
- `internal/query/engine_run.go` ~288 — JoinSystemBlocks에 inventorySystemBlocks 추가
- `cmd/argus/main.go` — server_inventory 도구 등록
- `internal/utils/permissions/classifier_decision.go:8-27` — `"server_inventory": true`
- `internal/memdir/` — settings.json `inventory.*` 필드 파싱 (옵션, 미구현 시 기본값 사용)

### PR1에서 변경하지 않음
- `internal/tui/modal_server_list.go` — 평문 emit 그대로 (PR2에서 통일)
- `internal/query/workspace_prompt.go` — 기존 inspect 주입 그대로 유지
- Oracle/WAS/MQ probe — PR2

---

## 검증 방법

### 단위 테스트
```bash
go test -v ./internal/services/inventory/...
go test -v ./internal/services/inventory/probes/...
go test -v ./internal/services/workspace/channel/...
go test -v ./internal/query/...
go test -v ./internal/tools/serverconnect/...
```

특히:
- `cache_test.go` — sha256 키, 메모리 + 디스크 2단 TTL, force vs disconnect 정책 검증
- `runner_test.go` — singleflight, epoch drop 검증 (fakeExec 모킹)
- `inventory_channel_test.go` — fresh ssh.Session 사용, 사용자 exec lane 미점유 검증
- cgroup parser 5종 fixture로 hosting 분류 검증
- ServerConnectRenderer 테스트: interactive 활성/비활성 + 마커 있음/없음 + 한국어/영어 메시지 모두 통과

### End-to-end
```bash
go fmt ./...
go vet ./...
go build -o Argus.exe ./cmd/argus

# 1. UI 빨간색 수정 검증
./Argus.exe --aidebug -p "/server connect a100-server"
# 출력의 본문이 빨간색이 아닌 정상 색으로 나오는지

# 2. 인벤토리 자동 수집 + LLM 주입 검증
sleep 6  # 백그라운드 인벤토리 5초 + 여유
./Argus.exe --aidebug -p "vLLM의 gemma 어디에 떠있는지" -r <session_id>
# system prompt(--aidebug 출력)에 [LLM Serving] 섹션 + hosting 매핑 포함 확인
# LLM 응답에 "k8s pod vllm-system/vllm-gemma" 또는 "docker container vllm-llama" 명시 확인

# 3. 디스크 캐시 검증
./Argus.exe --aidebug -p "exit"
./Argus.exe --aidebug -p "/server connect a100-server"  # 재실행
# 이번에는 인벤토리가 즉시(< 1초) 출력되는지 (디스크 캐시 hit)

# 4. /server scan 강제 갱신
./Argus.exe --aidebug -p "/server scan a100-server"
# 새 수집 트리거되는지

# 5. 동시 connect 중복 방지 (singleflight)
./Argus.exe --aidebug -p "/server connect a100-server" & \
./Argus.exe --aidebug -p "/server connect a100-server" & wait
# 동일 alias scan이 한 번만 실행됨 (debug log 또는 metrics)
```

---

## Critical Files

- [internal/services/workspace/manager.go](internal/services/workspace/manager.go) — Manager 통합 진입점
- [internal/services/workspace/inspect.go](internal/services/workspace/inspect.go) — marker-first 패턴 레퍼런스
- [internal/services/workspace/channel/metrics_channel.go](internal/services/workspace/channel/metrics_channel.go) — 별도 SSH session 패턴 레퍼런스
- [internal/services/workspace/channel/manager.go](internal/services/workspace/channel/manager.go) — AcquireMetrics 패턴
- [internal/tools/serverconnect/serverconnect.go](internal/tools/serverconnect/serverconnect.go) — 마커 prefix + 트리거 통일
- [internal/tools/serverconnect/ui.go](internal/tools/serverconnect/ui.go) — RenderToolResult 마커 기반 분기
- [internal/query/engine_run.go](internal/query/engine_run.go) — system prompt 조립
- [internal/query/workspace_prompt.go](internal/query/workspace_prompt.go) — 기존 inspect 주입 (참조용)
- [internal/constants/paths.go](internal/constants/paths.go) — `ConfigDir()` 기준
- [internal/tools/registry.go](internal/tools/registry.go) — `Lookup()` 사용
- [internal/utils/permissions/classifier_decision.go](internal/utils/permissions/classifier_decision.go) — safeAutoModeTools

---

## v1 → v2 변경 핵심 요약

1. **PR1 축소**: Oracle/WAS/MQ → PR2. PR1은 LLM Serving + Docker + K8s + UI 빨간색.
2. **빨간색 원인 확정 + 마커 기반 해결**: 영어/한국어 mismatch. `[ARGUS_SERVER_CONNECT:connected]` 마커.
3. **headless 안전성**: `theme.IsInteractive()` 분기로 정상 결과가 사라지지 않게.
4. **SSH 분리**: `PurposeInventory` 별도 channel. 사용자 lane 미점유.
5. **K8s 매핑 전략**: `kubectl get pods -o json` + `containerStatuses[].containerID` cross-reference. `/v1/models`는 native/docker만.
6. **Cache 정책**: disconnect 시 inflight 취소 + memory만, disk는 force/config-change/TTL만.
7. **경로**: `constants.ConfigDir()` + 신규 `InventoryCacheDir()`. `~/.argus` 사용 안 함.
8. **동시성 안전**: `singleflight` per-alias + `epoch` 검증으로 callback drop.
9. **모달 통일은 PR2로**: `Engine.InvokeToolForUI` 신설 + TUI config에 tool registry 노출.

---

## 참고한 외부 자료

- [How Claude Code Builds a System Prompt](https://www.dbreunig.com/2026/04/04/how-claude-code-builds-a-system-prompt.html)
- [Aider Repository Map](https://aider.chat/docs/repomap.html)
- [Building a better repository map with tree sitter — Aider](https://aider.chat/2023/10/22/repomap.html)
- [Warp Terminal and Agent modes](https://docs.warp.dev/agent-platform/local-agents/interacting-with-agents/terminal-and-agent-modes/)
- [Warp SSH features](https://docs.warp.dev/terminal/warpify/ssh/)
- [Ansible gather_facts module](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/gather_facts_module.html)
- [Ansible Facts — Spacelift](https://spacelift.io/blog/ansible-facts)
- [Osquery — Elastic Docs](https://www.elastic.co/docs/solutions/security/investigate/osquery)
- [Monitoring Docker containers with Osquery](https://zercurity.medium.com/monitoring-and-inspecting-docker-containers-images-with-osquery-2ae4e43a1b0b)
- [vLLM /info endpoint issue #5959](https://github.com/vllm-project/vllm/issues/5959)
- [opencode-local-setup — vLLM/Ollama auto sync](https://github.com/groxaxo/opencode-local-setup)
