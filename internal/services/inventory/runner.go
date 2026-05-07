package inventory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/koreaf16/argus/internal/services/inventory/probes"
)

const globalTimeout = 25 * time.Second

// InventoryPhase identifies each callback point during a streaming collection.
type InventoryPhase = string

const (
	PhaseHeader = "header" // system_header probe done; partial snapshot ready
	PhaseReady  = "ready"  // all probes done; full snapshot ready
	PhaseFailed = "failed" // scan failed (includes error)
)

// OnPhaseFunc is called at each phase of a streaming inventory collection.
type OnPhaseFunc func(phase InventoryPhase, snap InventorySnapshot, err error)

// ExecFunc runs a bash script on a remote server and returns stdout.
type ExecFunc func(ctx context.Context, script string) (string, error)

// Runner orchestrates probe collection for one server at a time.
type Runner struct {
	cache     *cache
	probeList []probes.Probe

	sf sfGroup // per-alias singleflight
}

// sfGroup is a minimal per-alias singleflight: only one scan per alias at a time.
type sfGroup struct {
	mu  sync.Mutex
	fly map[string]bool
}

func (g *sfGroup) try(alias string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fly[alias] {
		return false
	}
	if g.fly == nil {
		g.fly = make(map[string]bool)
	}
	g.fly[alias] = true
	return true
}

func (g *sfGroup) done(alias string) {
	g.mu.Lock()
	g.fly[alias] = false
	g.mu.Unlock()
}

// NewRunner creates a Runner with the standard probe set.
func NewRunner() *Runner {
	return &Runner{
		cache: newCache(),
		probeList: []probes.Probe{
			// Infrastructure
			&probes.DockerProbe{},
			&probes.KubernetesProbe{},
			// AI/ML
			&probes.LLMServingProbe{},
			// GPU
			&probes.GPUProbe{},
			// Databases
			&probes.OracleProbe{},
			&probes.DB2Probe{},
			&probes.GeneralDatabasesProbe{},
			// Web / App servers
			&probes.TomcatWildflyProbe{},
			&probes.NginxApacheProbe{},
			// Message queues
			&probes.KafkaZookeeperProbe{},
			&probes.RabbitmqActivemqNatsProbe{},
		},
	}
}

// Cache returns the underlying cache (for GetInventorySnapshot calls).
func (r *Runner) Cache() *cache { return r.cache }

// CollectAsync starts a background scan for alias. onDone is called with the
// result when the scan completes. If a scan for the same alias is already
// in progress the call is a no-op.
func (r *Runner) CollectAsync(ctx context.Context, alias string, execFn ExecFunc, onDone func(InventorySnapshot)) {
	if !r.sf.try(alias) {
		return
	}
	r.cache.Set(alias, InventorySnapshot{Alias: alias, Status: StatusPending, CollectedAt: time.Now()})
	go func() {
		defer r.sf.done(alias)
		r.CollectStreaming(ctx, alias, execFn, func(phase InventoryPhase, snap InventorySnapshot, _ error) {
			if phase == PhaseReady || phase == PhaseFailed {
				if onDone != nil {
					onDone(snap)
				}
			}
		})
	}()
}

// CollectSync runs the scan synchronously and stores the result in cache.
// Pass force=true to bypass the cache.
func (r *Runner) CollectSync(ctx context.Context, alias string, execFn ExecFunc, force bool) (InventorySnapshot, error) {
	if !force {
		if snap, ok := r.cache.Get(alias); ok && snap.Status != StatusPending {
			return snap, nil
		}
	}
	var result InventorySnapshot
	r.CollectStreaming(ctx, alias, execFn, func(phase InventoryPhase, snap InventorySnapshot, _ error) {
		if phase == PhaseReady || phase == PhaseFailed {
			result = snap
		}
	})
	return result, nil
}

// CollectStreaming runs the collection in two phases:
//  1. system_header probe only (fast, ~3s) → onPhase(PhaseHeader, partial, nil)
//  2. remaining probes in parallel via errgroup → onPhase(PhaseReady, full, nil)
//
// On system_header failure → onPhase(PhaseFailed, empty, err) and returns immediately.
// This call blocks until PhaseReady or PhaseFailed. onPhase may be nil.
func (r *Runner) CollectStreaming(ctx context.Context, alias string, execFn ExecFunc, onPhase OnPhaseFunc) {
	totalStart := time.Now()

	// Phase 1: system_header (fast, ~3s)
	var hdrProbe probes.SystemHeaderProbe
	hdr, err := hdrProbe.RunDirect(ctx, probes.ProbeExec(execFn))
	if err != nil {
		failed := InventorySnapshot{
			Alias:       alias,
			CollectedAt: totalStart,
			Status:      StatusFailed,
			DurationMs:  time.Since(totalStart).Milliseconds(),
			Errors:      map[string]string{"system_header": err.Error()},
		}
		r.cache.Set(alias, failed)
		if onPhase != nil {
			onPhase(PhaseFailed, failed, err)
		}
		return
	}

	partial := InventorySnapshot{
		Alias:       alias,
		CollectedAt: totalStart,
		Status:      StatusPartial,
		System:      systemHeaderToInfo(hdr),
	}
	r.cache.Set(alias, partial)
	if onPhase != nil {
		onPhase(PhaseHeader, partial, nil)
	}

	// Phase 2: remaining probes in parallel
	rest := r.collectRestParallel(ctx, alias, execFn)
	rest.System = partial.System
	rest.DurationMs = time.Since(totalStart).Milliseconds()
	r.cache.Set(alias, rest)
	if onPhase != nil {
		onPhase(PhaseReady, rest, nil)
	}
}

// CollectHeader runs only the system_header probe and caches the partial result.
// Used for fast startup system info without blocking on the full scan.
func (r *Runner) CollectHeader(ctx context.Context, alias string, execFn ExecFunc) {
	var hdrProbe probes.SystemHeaderProbe
	hdr, err := hdrProbe.RunDirect(ctx, probes.ProbeExec(execFn))
	if err != nil {
		return
	}
	snap := InventorySnapshot{
		Alias:       alias,
		CollectedAt: time.Now(),
		Status:      StatusPartial,
		System:      systemHeaderToInfo(hdr),
	}
	r.cache.Set(alias, snap)
}

// collectRestParallel runs all probeList probes in parallel using errgroup.
// Per-probe errors are stored in the returned snapshot; the function itself
// never returns a non-nil error.
func (r *Runner) collectRestParallel(ctx context.Context, alias string, execFn ExecFunc) InventorySnapshot {
	start := time.Now()

	gctx, cancel := context.WithTimeout(ctx, globalTimeout)
	defer cancel()

	eg, egCtx := errgroup.WithContext(gctx)
	eg.SetLimit(6) // SSH MaxSessions protection (11 probes total, MaxSessions default=10)

	results := make([]probes.Result, len(r.probeList))
	errMap := make(map[string]string)
	var errMu sync.Mutex

	for i, pr := range r.probeList {
		eg.Go(func() error {
			pCtx, pCancel := context.WithTimeout(egCtx, pr.PreferredTimeout())
			defer pCancel()
			res, runErr := pr.Run(pCtx, probes.ProbeExec(execFn))
			if runErr != nil {
				errMu.Lock()
				errMap[pr.Name()] = runErr.Error()
				errMu.Unlock()
				return nil // don't cancel sibling probes
			}
			results[i] = res
			return nil
		})
	}
	_ = eg.Wait()

	snap := InventorySnapshot{
		Alias:       alias,
		CollectedAt: start,
	}
	// Apply in probeList order for deterministic LLM hosting resolution
	// (llm_serving needs docker/k8s results already in snap).
	for _, res := range results {
		applyResult(&snap, res)
	}
	snap.DurationMs = time.Since(start).Milliseconds()
	if len(errMap) == 0 {
		snap.Status = StatusReady
	} else {
		snap.Status = StatusPartial
		snap.Errors = errMap
	}
	return snap
}

func systemHeaderToInfo(hdr *probes.SystemHeaderResult) *SystemInfo {
	return &SystemInfo{
		OS:            hdr.OS,
		OSPretty:      hdr.OSPretty,
		User:          hdr.User,
		Shell:         hdr.Shell,
		Uptime:        hdr.Uptime,
		Memory:        hdr.Memory,
		Disk:          hdr.Disk,
		ListenerCount: hdr.ListenerCount,
		ServiceCount:  hdr.ServiceCount,
	}
}

func applyResult(snap *InventorySnapshot, res probes.Result) {
	if res.SystemHeader != nil {
		snap.System = systemHeaderToInfo(res.SystemHeader)
	}
	if res.Docker != nil {
		for _, c := range res.Docker {
			snap.Containers = append(snap.Containers, ContainerInfo{
				ID:      c.ID,
				Name:    c.Name,
				Image:   c.Image,
				State:   c.State,
				Ports:   c.Ports,
				Command: c.Command,
			})
		}
	}
	if res.Kubernetes != nil {
		k := res.Kubernetes
		info := &K8sInfo{
			Context:    k.Context,
			Permission: k.Permission,
			NodeCount:  len(k.Nodes),
			PodCount:   len(k.Pods),
			Namespaces: k.Namespaces,
		}
		for _, pod := range k.Pods {
			info.Workloads = append(info.Workloads, K8sWorkload{
				Namespace:   pod.Namespace,
				Name:        pod.Name,
				Phase:       pod.Phase,
				Image:       pod.Image,
				NodeName:    pod.NodeName,
				ContainerID: pod.ContainerID,
			})
		}
		snap.Kubernetes = info
	}
	for _, llm := range res.LLMServing {
		serving := LLMServingInfo{
			Engine:   llm.Engine,
			Port:     llm.Port,
			Endpoint: fmt.Sprintf("http://localhost:%d", llm.Port),
			Hosting:  resolveLLMHosting(llm, snap),
		}
		for _, m := range llm.Models {
			serving.Models = append(serving.Models, LLMModel{
				ID:     m,
				Family: extractFamily(m),
			})
		}
		snap.LLMServing = append(snap.LLMServing, serving)
	}
	for _, db := range res.Databases {
		snap.Databases = append(snap.Databases, DatabaseInfo{
			Engine:     db.Engine,
			Version:    db.Version,
			Instances:  db.Instances,
			Listeners:  db.Listeners,
			Confidence: db.Confidence,
			Evidence:   db.Evidence,
		})
	}
	for _, was := range res.WAS {
		snap.WAS = append(snap.WAS, WASInfo{
			Engine:     was.Engine,
			Version:    was.Version,
			Home:       was.Home,
			Ports:      was.Ports,
			Confidence: was.Confidence,
			Evidence:   was.Evidence,
		})
	}
	for _, mq := range res.MQ {
		snap.MQ = append(snap.MQ, MQInfo{
			Engine:     mq.Engine,
			Version:    mq.Version,
			Ports:      mq.Ports,
			Confidence: mq.Confidence,
			Evidence:   mq.Evidence,
		})
	}
	if res.GPU != nil && snap.GPU == nil {
		gpu := &GPUInfo{CUDADriver: res.GPU.CUDADriver}
		for _, d := range res.GPU.Devices {
			gpu.Devices = append(gpu.Devices, GPUDevice{
				Name:          d.Name,
				DriverVersion: d.DriverVersion,
				MemoryMB:      d.MemoryMB,
			})
		}
		snap.GPU = gpu
	}
}

// resolveLLMHosting determines whether an LLM engine runs in k8s, docker, or natively.
func resolveLLMHosting(llm probes.LLMServingResult, snap *InventorySnapshot) LLMHosting {
	cg := llm.Cgroup
	if cg == "" {
		return LLMHosting{Type: "unknown", Pid: llm.CgroupPid}
	}

	if strings.Contains(cg, "kubepods") {
		podName := ""
		if snap.Kubernetes != nil {
			for _, w := range snap.Kubernetes.Workloads {
				if w.ContainerID != "" && strings.Contains(cg, w.ContainerID) {
					podName = w.Namespace + "/" + w.Name
					break
				}
			}
		}
		return LLMHosting{Type: "k8s", K8sPod: podName, Pid: llm.CgroupPid}
	}

	if strings.Contains(cg, "docker") || strings.Contains(cg, "containerd") {
		containerName := ""
		for _, c := range snap.Containers {
			if strings.Contains(cg, c.ID) {
				containerName = c.Name
				break
			}
		}
		return LLMHosting{Type: "docker", Container: containerName, Pid: llm.CgroupPid}
	}

	if strings.Contains(cg, "system.slice") || strings.Contains(cg, "user.slice") {
		return LLMHosting{Type: "systemd", Pid: llm.CgroupPid}
	}

	return LLMHosting{Type: "native", Pid: llm.CgroupPid}
}

func extractFamily(modelID string) string {
	lower := strings.ToLower(modelID)
	for _, family := range []string{"gemma", "llama", "qwen", "mistral", "mixtral", "phi", "falcon", "gpt", "claude", "yi", "deepseek"} {
		if strings.Contains(lower, family) {
			return family
		}
	}
	return ""
}
