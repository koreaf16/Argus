package probes

import (
	"context"
	"strings"
	"time"
)

// KubernetesProbe detects Kubernetes cluster state via kubectl.
type KubernetesProbe struct{}

func (p *KubernetesProbe) Name() string { return "kubernetes" }

func (p *KubernetesProbe) ScriptFragment() string {
	return `
if ! command -v kubectl >/dev/null 2>&1; then exit 0; fi
echo "CTX:$(kubectl config current-context 2>/dev/null)"
echo "PERM:$(kubectl auth can-i list pods --all-namespaces 2>/dev/null)"
kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null | sed 's/^/NODE:/'
kubectl get pods -A -o jsonpath='{range .items[*]}{.metadata.namespace}{"\t"}{.metadata.name}{"\t"}{.status.phase}{"\t"}{.spec.containers[0].image}{"\t"}{.spec.nodeName}{"\t"}{.status.containerStatuses[0].containerID}{"\n"}{end}' 2>/dev/null | head -50 | sed 's/^/POD:/'
kubectl get svc -A -o jsonpath='{range .items[*]}{.metadata.namespace}{"\t"}{.metadata.name}{"\t"}{.spec.type}{"\t"}{.spec.ports[0].port}{"\n"}{end}' 2>/dev/null | head -30 | sed 's/^/SVC:/'
kubectl get namespaces -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null | sed 's/^/NS:/'
`
}

func (p *KubernetesProbe) PreferredTimeout() time.Duration { return 8 * time.Second }

func (p *KubernetesProbe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, p.ScriptFragment())
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

func (p *KubernetesProbe) Parse(stdout string) (Result, error) {
	r := &K8sResult{Permission: "unknown"}
	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "CTX:"):
			r.Context = strings.TrimPrefix(line, "CTX:")
		case strings.HasPrefix(line, "PERM:"):
			v := strings.TrimPrefix(line, "PERM:")
			if strings.TrimSpace(v) == "yes" {
				r.Permission = "admin"
			} else {
				r.Permission = "limited"
			}
		case strings.HasPrefix(line, "NODE:"):
			n := strings.TrimPrefix(line, "NODE:")
			if n != "" {
				r.Nodes = append(r.Nodes, n)
			}
		case strings.HasPrefix(line, "NS:"):
			ns := strings.TrimPrefix(line, "NS:")
			if ns != "" {
				r.Namespaces = append(r.Namespaces, ns)
			}
		case strings.HasPrefix(line, "POD:"):
			parts := strings.Split(strings.TrimPrefix(line, "POD:"), "\t")
			if len(parts) < 4 {
				continue
			}
			pod := K8sPod{
				Namespace: parts[0],
				Name:      parts[1],
				Phase:     parts[2],
				Image:     parts[3],
			}
			if len(parts) >= 5 {
				pod.NodeName = parts[4]
			}
			if len(parts) >= 6 {
				// containerd://abc123… or docker://abc123…
				cid := parts[5]
				if idx := strings.LastIndex(cid, "//"); idx >= 0 {
					cid = cid[idx+2:]
				}
				if len(cid) > 12 {
					cid = cid[:12]
				}
				pod.ContainerID = cid
			}
			r.Pods = append(r.Pods, pod)
		case strings.HasPrefix(line, "SVC:"):
			parts := strings.Split(strings.TrimPrefix(line, "SVC:"), "\t")
			if len(parts) < 4 {
				continue
			}
			r.Services = append(r.Services, K8sService{
				Namespace: parts[0],
				Name:      parts[1],
				Type:      parts[2],
				Port:      parts[3],
			})
		}
	}
	if r.Context == "" && len(r.Nodes) == 0 {
		return Result{}, nil
	}
	return Result{Kubernetes: r}, nil
}
