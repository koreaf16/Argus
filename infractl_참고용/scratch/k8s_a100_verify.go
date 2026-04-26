//go:build tools
// +build tools

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	sshconn "github.com/yourorg/infractl/internal/connector/ssh"
	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/privilege"
	"github.com/yourorg/infractl/internal/store"
	"github.com/yourorg/infractl/internal/tools"
)

type testResult struct {
	Name    string
	Passed  bool
	Command string
	Output  string
	Note    string
}

type podCandidate struct {
	Namespace string
	Name      string
	Container string
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	dbPath := filepath.Join("bin", ".infractl", "infractl.db")
	serverName := "a100"
	if len(os.Args) > 1 && strings.TrimSpace(os.Args[1]) != "" {
		dbPath = os.Args[1]
	}
	if len(os.Args) > 2 && strings.TrimSpace(os.Args[2]) != "" {
		serverName = os.Args[2]
	}

	absDB, err := filepath.Abs(dbPath)
	if err != nil {
		log.Fatalf("resolve db path: %v", err)
	}
	logPath := filepath.Join("scratch", fmt.Sprintf("k8s_a100_verify_%s.log", time.Now().Format("20060102_150405")))
	logFile, err := os.Create(logPath)
	if err != nil {
		log.Fatalf("create log: %v", err)
	}
	defer logFile.Close()
	out := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		fmt.Print(msg)
		_, _ = logFile.WriteString(msg)
	}

	st, err := store.NewSQLiteStore(ctx, absDB)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	srv, err := findServer(ctx, st, serverName)
	if err != nil {
		log.Fatal(err)
	}
	exec := newSSHExecutor(srv)
	defer exec.Close()

	k8s := &tools.K8sQueryTool{}
	shell := &tools.ShellExecTool{
		PrivilegeCache: privilege.NewCache(),
		PromptHandler:  storePrivilegeHandler{st: st},
	}

	var results []testResult
	out("[setup] db=%s server=%s host=%s user=%s log=%s\n", absDB, srv.Name, srv.Host, srv.User, logPath)

	protected := runRaw(ctx, exec, `kubectl get pods,deployments,statefulsets,daemonsets,services -A -o wide 2>/dev/null | grep -Ei 'vllm|llama|llama.cpp|llamacpp' || true`)
	out("\n[protected candidates: vllm/llama]\n%s\n", trimOrNone(protected.Stdout))

	results = append(results, k8sRun(ctx, k8s, exec, "get nodes wide", map[string]any{
		"action": "get", "resource": "nodes", "output": "wide", "target": serverName,
	}))
	results = append(results, k8sRun(ctx, k8s, exec, "get namespaces", map[string]any{
		"action": "get", "resource": "namespaces", "output": "wide", "target": serverName,
	}))
	results = append(results, k8sRun(ctx, k8s, exec, "multi get pods/services/deployments", map[string]any{
		"action": "get", "resources": []any{"pods", "services", "deployments"}, "namespace": "all", "output": "wide", "target": serverName,
	}))
	results = append(results, k8sRun(ctx, k8s, exec, "get events", map[string]any{
		"action": "get", "resource": "events", "namespace": "all", "output": "wide", "target": serverName,
	}))

	pod := choosePod(ctx, exec)
	if pod.Name != "" {
		results = append(results, k8sRun(ctx, k8s, exec, "describe safe pod", map[string]any{
			"action": "describe", "resource": "pod", "name": pod.Name, "namespace": pod.Namespace, "target": serverName,
		}))
		results = append(results, k8sRun(ctx, k8s, exec, "logs safe pod", map[string]any{
			"action": "logs", "name": pod.Name, "namespace": pod.Namespace, "tail_lines": 20, "target": serverName,
		}))
	} else {
		results = append(results, testResult{Name: "describe/logs safe pod", Passed: false, Note: "non-vllm/non-llama running single-container pod not found"})
	}
	results = append(results, k8sRun(ctx, k8s, exec, "top nodes", map[string]any{
		"action": "top", "resource": "nodes", "target": serverName,
	}))

	ns := fmt.Sprintf("infractl-k8s-test-%s", time.Now().Format("150405"))
	cleanup := true
	results = append(results, shellRun(ctx, shell, exec, "create test namespace", fmt.Sprintf("kubectl create namespace %s", ns)))
	defer func() {
		if cleanup {
			res := runRaw(context.Background(), exec, fmt.Sprintf("kubectl delete namespace %s --ignore-not-found=true --wait=false", ns))
			out("\n[cleanup] kubectl delete namespace %s exit=%d\n%s\n%s\n", ns, res.ExitCode, res.Stdout, res.Stderr)
		}
	}()

	results = append(results, shellRun(ctx, shell, exec, "create labeled configmap", fmt.Sprintf("kubectl -n %s create configmap infractl-k8s-cm --from-literal=probe=ok", ns)))
	results = append(results, shellRun(ctx, shell, exec, "label configmap", fmt.Sprintf("kubectl -n %s label configmap infractl-k8s-cm infractl-test=true", ns)))
	results = append(results, k8sRun(ctx, k8s, exec, "get configmap by selector", map[string]any{
		"action": "get", "resource": "configmaps", "namespace": ns, "selector": "infractl-test=true", "output": "wide", "target": serverName,
	}))

	results = append(results, shellRun(ctx, shell, exec, "create pause deployment", fmt.Sprintf("kubectl -n %s create deployment infractl-k8s-deploy --image=registry.k8s.io/pause:3.9 --replicas=1", ns)))
	results = append(results, k8sRun(ctx, k8s, exec, "get test deployment", map[string]any{
		"action": "get", "resource": "deployments", "namespace": ns, "output": "wide", "target": serverName,
	}))
	results = append(results, shellRun(ctx, shell, exec, "scale deployment down", fmt.Sprintf("kubectl -n %s scale deployment/infractl-k8s-deploy --replicas=0", ns)))
	results = append(results, k8sRun(ctx, k8s, exec, "verify deployment scaled down", map[string]any{
		"action": "get", "resource": "deployments", "name": "infractl-k8s-deploy", "namespace": ns, "output": "wide", "target": serverName,
	}))
	results = append(results, shellRun(ctx, shell, exec, "scale deployment up", fmt.Sprintf("kubectl -n %s scale deployment/infractl-k8s-deploy --replicas=1", ns)))
	results = append(results, k8sRun(ctx, k8s, exec, "describe test deployment", map[string]any{
		"action": "describe", "resource": "deployment", "name": "infractl-k8s-deploy", "namespace": ns, "target": serverName,
	}))

	missingName := k8sRun(ctx, k8s, exec, "logs missing name validation", map[string]any{
		"action": "logs", "namespace": ns, "target": serverName,
	})
	results = append(results, missingName)

	out("\n[summary]\n")
	sort.SliceStable(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	for _, r := range results {
		status := "PASS"
		if !r.Passed {
			status = "FAIL"
		}
		out("- %s %s\n", status, r.Name)
		if r.Command != "" {
			out("  command/tool: %s\n", r.Command)
		}
		if r.Note != "" {
			out("  note: %s\n", r.Note)
		}
		out("  output:\n%s\n", indent(truncate(r.Output, 4000)))
	}
	out("\n[done] log=%s\n", logPath)
}

func findServer(ctx context.Context, st store.ServerStore, name string) (store.Server, error) {
	servers, err := st.List(ctx)
	if err != nil {
		return store.Server{}, err
	}
	for _, srv := range servers {
		if strings.EqualFold(srv.Name, name) {
			return srv, nil
		}
	}
	return store.Server{}, fmt.Errorf("server %q not found", name)
}

func newSSHExecutor(srv store.Server) *sshconn.SSHExecutor {
	cfg := &sshconn.Config{
		Host:         srv.Host,
		Port:         srv.Port,
		User:         srv.User,
		AuthType:     string(srv.AuthType),
		WorkspaceDir: srv.WorkspaceDir,
	}
	if srv.AuthType == store.AuthTypeKey {
		cfg.KeyPath = srv.Credential
	} else {
		cfg.Password = srv.Credential
	}
	return sshconn.NewSSHExecutor(srv.Name, sshconn.NewClient(cfg), srv.OS, srv.WorkspaceDir)
}

func k8sRun(ctx context.Context, tool *tools.K8sQueryTool, exec executor.Executor, name string, args map[string]any) testResult {
	out, err := tool.Execute(ctx, args, exec)
	content := out.Content
	passed := err == nil && !strings.Contains(content, "kubectl error") && !strings.Contains(content, "execution failed")
	note := ""
	if strings.Contains(content, "Metrics API not available") || strings.Contains(content, "metrics.k8s.io") {
		note = "metrics-server/top unavailable"
	}
	if strings.Contains(strings.ToLower(content), "error: resource type is required for get action") {
		note = "validation message is misleading for non-get action"
	}
	return testResult{Name: name, Passed: passed, Command: "k8s_query " + mustJSON(args), Output: content, Note: note}
}

func shellRun(ctx context.Context, tool *tools.ShellExecTool, exec executor.Executor, name, cmd string) testResult {
	out, err := tool.Execute(ctx, map[string]any{"command": cmd, "target": exec.Target(), "reason": name}, exec)
	passed := err == nil && out.Success
	return testResult{Name: name, Passed: passed, Command: cmd, Output: out.Content}
}

func runRaw(ctx context.Context, exec executor.Executor, cmd string) executor.ExecResult {
	res, err := exec.Execute(ctx, cmd)
	if err != nil {
		return executor.ExecResult{ExitCode: 1, Stderr: err.Error()}
	}
	return res
}

func choosePod(ctx context.Context, exec executor.Executor) podCandidate {
	res := runRaw(ctx, exec, "kubectl get pods -A -o jsonpath='{range .items[*]}{.metadata.namespace}{\"\\t\"}{.metadata.name}{\"\\t\"}{.status.phase}{\"\\t\"}{range .spec.containers[*]}{.name}{\":\"}{.image}{\",\"}{end}{\"\\n\"}{end}'")
	if res.ExitCode != 0 {
		return podCandidate{}
	}
	for _, line := range strings.Split(res.Stdout, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 4 || fields[2] != "Running" {
			continue
		}
		containers := strings.Split(strings.Trim(fields[3], ","), ",")
		if len(containers) != 1 {
			continue
		}
		containerParts := strings.SplitN(containers[0], ":", 2)
		if len(containerParts) != 2 {
			continue
		}
		haystack := strings.ToLower(fields[0] + " " + fields[1] + " " + containerParts[1])
		if strings.Contains(haystack, "vllm") || strings.Contains(haystack, "llama") {
			continue
		}
		return podCandidate{Namespace: fields[0], Name: fields[1], Container: containerParts[0]}
	}
	return podCandidate{}
}

type storePrivilegeHandler struct {
	st store.ServerStore
}

func (h storePrivilegeHandler) RequestPassword(ctx context.Context, req privilege.PromptRequest) (privilege.PromptResponse, error) {
	servers, err := h.st.List(ctx)
	if err != nil {
		return privilege.PromptResponse{}, err
	}
	for _, srv := range servers {
		if srv.AuthType != store.AuthTypePassword || srv.Credential == "" {
			continue
		}
		if strings.EqualFold(srv.Name, req.Target) || strings.EqualFold(srv.Host, req.Target) {
			return privilege.PromptResponse{Password: srv.Credential}, nil
		}
	}
	return privilege.PromptResponse{}, fmt.Errorf("stored password not found for %s", req.Target)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if max > 0 && len(s) > max {
		return s[:max] + "\n[... truncated ...]"
	}
	return s
}

func indent(s string) string {
	if strings.TrimSpace(s) == "" {
		return "    (no output)"
	}
	return "    " + strings.ReplaceAll(s, "\n", "\n    ")
}

func trimOrNone(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "(none)"
	}
	return s
}
