package inventory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/koreaf16/argus/internal/services/inventory/probes"
)

const (
	markerFmt  = "<<ARGUS_INV:%s>>"
	globalTimeout = 25 * time.Second
)

// ExecFunc runs a bash script on a remote server and returns stdout.
type ExecFunc func(ctx context.Context, script string) (string, error)

// Runner orchestrates probe collection for one server at a time.
type Runner struct {
	cache  *cache
	probeList []probes.Probe

	sf   sfGroup // per-alias singleflight
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
			&probes.DockerProbe{},
			&probes.KubernetesProbe{},
			&probes.LLMServingProbe{},
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
	// Store pending snapshot immediately so prompt injection can say "收집 중".
	r.cache.Set(alias, InventorySnapshot{Alias: alias, Status: StatusPending, CollectedAt: time.Now()})
	go func() {
		defer r.sf.done(alias)
		snap, _ := r.collect(ctx, alias, execFn)
		r.cache.Set(alias, snap)
		if onDone != nil {
			onDone(snap)
		}
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
	snap, err := r.collect(ctx, alias, execFn)
	if err == nil {
		r.cache.Set(alias, snap)
	}
	return snap, err
}

func (r *Runner) collect(ctx context.Context, alias string, execFn ExecFunc) (InventorySnapshot, error) {
	start := time.Now()
	snap := InventorySnapshot{
		Alias:       alias,
		CollectedAt: start,
		Errors:      make(map[string]string),
	}

	gctx, cancel := context.WithTimeout(ctx, globalTimeout)
	defer cancel()

	script := r.buildScript()
	raw, err := execFn(gctx, script)
	if err != nil {
		snap.Status = StatusFailed
		snap.Errors["exec"] = err.Error()
		snap.DurationMs = time.Since(start).Milliseconds()
		return snap, err
	}

	sections := splitSections(raw, r.probeList)
	allOK := true
	for i, pr := range r.probeList {
		section := sections[i]
		res, parseErr := pr.Parse(section)
		if parseErr != nil {
			snap.Errors[pr.Name()] = parseErr.Error()
			allOK = false
			continue
		}
		applyResult(&snap, res)
	}

	snap.DurationMs = time.Since(start).Milliseconds()
	if allOK {
		snap.Status = StatusReady
	} else {
		snap.Status = StatusPartial
	}
	if len(snap.Errors) == 0 {
		snap.Errors = nil
	}
	return snap, nil
}

func (r *Runner) buildScript() string {
	var sb strings.Builder
	sb.WriteString("set +e\n")
	for _, pr := range r.probeList {
		fmt.Fprintf(&sb, "printf '%s\\n'\n", fmt.Sprintf(markerFmt, pr.Name()))
		sb.WriteString(pr.ScriptFragment())
		sb.WriteString("\n")
	}
	fmt.Fprintf(&sb, "printf '%s\\n'\n", fmt.Sprintf(markerFmt, "END"))
	return sb.String()
}

func splitSections(raw string, probeList []probes.Probe) []string {
	sections := make([]string, len(probeList))
	for i, pr := range probeList {
		startMark := fmt.Sprintf(markerFmt, pr.Name())
		startIdx := strings.Index(raw, startMark)
		if startIdx < 0 {
			continue
		}
		startIdx += len(startMark)

		var endMark string
		if i+1 < len(probeList) {
			endMark = fmt.Sprintf(markerFmt, probeList[i+1].Name())
		} else {
			endMark = fmt.Sprintf(markerFmt, "END")
		}
		endIdx := strings.Index(raw[startIdx:], endMark)
		if endIdx < 0 {
			sections[i] = strings.TrimSpace(raw[startIdx:])
		} else {
			sections[i] = strings.TrimSpace(raw[startIdx : startIdx+endIdx])
		}
	}
	return sections
}

func applyResult(snap *InventorySnapshot, res probes.Result) {
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
}

// resolveLLMHosting determines whether an LLM engine runs in k8s, docker, or natively.
func resolveLLMHosting(llm probes.LLMServingResult, snap *InventorySnapshot) LLMHosting {
	cg := llm.Cgroup
	if cg == "" {
		return LLMHosting{Type: "unknown", Pid: llm.CgroupPid}
	}

	if strings.Contains(cg, "kubepods") {
		// Try to find the matching pod in k8s results by container ID
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
		// Extract container ID from cgroup path
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
