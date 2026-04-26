// Package tools
// File: k8s_query.go
// Description: Kubernetes 클러스터 리소스 조회 전용 도구
// Responsibility: kubectl get/describe/logs 등 k8s 조회 명령을 단일 도구로 대체

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/yourorg/infractl/internal/executor"
)

// K8sQueryTool은 Kubernetes 클러스터 리소스를 조회한다.
type K8sQueryTool struct{}

func (t *K8sQueryTool) Name() string { return "k8s_query" }

func (t *K8sQueryTool) Description() string {
	return "Query Kubernetes cluster resources (pods, services, deployments, nodes, events, logs).\n" +
		"Use this INSTEAD of shell_exec with 'kubectl get', 'kubectl describe', 'kubectl logs', " +
		"'kubectl top' commands.\n" +
		"Requires kubectl configured on the target. Supports namespace filtering, label selectors, and multi-resource parallel queries."
}

func (t *K8sQueryTool) IsReadOnly() bool { return true }
func (t *K8sQueryTool) IsEnabled() bool  { return true }

func (t *K8sQueryTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "Action: get (list resources), describe (detail), logs (pod logs), top (resource usage). Default: get.",
				"enum":        []string{"get", "describe", "logs", "top"},
				"default":     "get",
			},
			"resource": map[string]interface{}{
				"type":        "string",
				"description": "Resource type: pods, services, deployments, nodes, events, namespaces, configmaps, ingress, statefulsets, daemonsets, jobs, pv, pvc. For get/top, comma-separated values are allowed (e.g. 'pods,services').",
			},
			"resources": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "Optional list of resource types for get/top actions. When multiple resources are provided, queries execute in parallel.",
			},
			"name": map[string]interface{}{
				"type":        "string",
				"description": "Specific resource name (optional). Required for describe and logs actions.",
			},
			"namespace": map[string]interface{}{
				"type":        "string",
				"description": "Namespace. Use 'all' for --all-namespaces. Omit for current namespace.",
			},
			"output": map[string]interface{}{
				"type":        "string",
				"description": "Output format: wide, yaml, json, name. Default: wide.",
				"enum":        []string{"wide", "yaml", "json", "name"},
				"default":     "wide",
			},
			"selector": map[string]interface{}{
				"type":        "string",
				"description": "Label selector (e.g. 'app=nginx,tier=frontend'). Optional.",
			},
			"tail_lines": map[string]interface{}{
				"type":        "integer",
				"description": "Number of log lines for logs action (default: 50).",
				"default":     50,
			},
			"target": map[string]interface{}{
				"type":        "string",
				"description": "Target server name. Omit to use the active workspace. Use 'localhost' ONLY for the local controller machine.",
			},
		},
	}
}

func (t *K8sQueryTool) Execute(ctx context.Context, args map[string]interface{}, exec executor.Executor) (ToolOutcome, error) {
	action, _ := argString(args, "action", false)
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		action = "get"
	}
	resource, _ := argString(args, "resource", false)
	resources := parseK8sResourceList(args, resource)
	name, _ := argString(args, "name", false)
	namespace, _ := argString(args, "namespace", false)
	output, _ := argString(args, "output", false)
	selector, _ := argString(args, "selector", false)
	tailLines := argInt(args, "tail_lines", 50)

	if action == "get" && len(resources) == 0 {
		// resource 없이 get 호출 시 클러스터 상태 overview 기본값 사용
		resources = []string{"nodes", "pods", "services"}
	}

	if action == "top" && len(resources) == 0 {
		resources = []string{"pods"}
	}

	if (action == "get" || action == "top") && len(resources) > 1 {
		out, ok := executeK8sMultiResource(ctx, exec, action, resources, name, namespace, output, selector, tailLines)
		return k8sOutcome(out, ok), nil
	}
	if len(resources) > 0 {
		resource = resources[0]
	}

	cmd := buildK8sCommand(action, resource, name, namespace, output, selector, tailLines)
	if cmd == "" {
		return k8sOutcome(k8sValidationError(action, resource, name), false), nil
	}

	out, ok := runK8sCommand(ctx, exec, cmd)
	return k8sOutcome(out, ok), nil
}

func executeK8sMultiResource(
	ctx context.Context,
	exec executor.Executor,
	action string,
	resources []string,
	name, namespace, output, selector string,
	tailLines int,
) (string, bool) {
	type resourceResult struct {
		out string
		ok  bool
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results = make(map[string]resourceResult, len(resources))
	)

	slog.Info("k8s_query_multi_start", "action", action, "count", len(resources))
	for _, rawResource := range resources {
		resource := rawResource
		wg.Add(1)
		go func() {
			defer wg.Done()
			taskID := fmt.Sprintf("%s_%s", action, resource)
			EmitOutput(ctx, fmt.Sprintf("task_start task=%s", taskID))
			start := time.Now()

			cmd := buildK8sCommand(action, resource, name, namespace, output, selector, tailLines)
			out, ok := runK8sCommand(ctx, exec, cmd)

			mu.Lock()
			results[resource] = resourceResult{out: out, ok: ok}
			mu.Unlock()

			dur := time.Since(start)
			if ok {
				EmitOutput(ctx, fmt.Sprintf("task_done task=%s dur=%s", taskID, dur))
				slog.Info("task_done", "task", taskID, "dur", dur)
				return
			}
			EmitOutput(ctx, fmt.Sprintf("task_fail task=%s dur=%s", taskID, dur))
			slog.Info("task_fail", "task", taskID, "dur", dur)
		}()
	}
	wg.Wait()

	var sections []string
	allOK := true
	for _, resource := range resources {
		res, ok := results[resource]
		if !ok {
			continue
		}
		if !res.ok {
			allOK = false
		}
		body := strings.TrimSpace(res.out)
		if body == "" {
			body = "(no output)"
		}
		sections = append(sections, fmt.Sprintf("[%s]\n%s", resource, body))
	}
	return strings.Join(sections, "\n\n"), allOK
}

func runK8sCommand(ctx context.Context, exec executor.Executor, cmd string) (string, bool) {
	result, err := exec.Execute(ctx, cmd)
	if err != nil {
		return fmt.Sprintf("execution failed: %s", err), false
	}
	out := strings.TrimSpace(result.Stdout)
	stderr := strings.TrimSpace(result.Stderr)
	if result.ExitCode != 0 {
		detail := strings.TrimSpace(out + "\n" + stderr)
		if detail == "" {
			detail = "(no output)"
		}
		return fmt.Sprintf("kubectl error (exit %d):\n%s", result.ExitCode, detail), false
	}

	if stderr != "" {
		if out != "" {
			out += "\n"
		}
		out += "[stderr]\n" + stderr
	}
	return out, true
}

func k8sOutcome(content string, ok bool) ToolOutcome {
	out := ToolOutcome{Content: content, Success: ok}
	if !ok {
		out.ExitCode = 1
		out.ErrorMessage = strings.TrimSpace(content)
	}
	return out
}

func k8sValidationError(action, resource, name string) string {
	switch action {
	case "logs":
		return "error: pod name is required for logs action"
	case "describe":
		if strings.TrimSpace(resource) == "" {
			return "error: resource type is required for describe action"
		}
		if strings.TrimSpace(name) == "" {
			return "error: resource name is required for describe action"
		}
		return "error: invalid describe request"
	case "top":
		return "error: resource type is required for top action"
	default:
		return "error: resource type is required for get action"
	}
}

func parseK8sResourceList(args map[string]interface{}, primary string) []string {
	seen := make(map[string]bool)
	ordered := make([]string, 0, 4)
	add := func(raw string) {
		for _, token := range splitResourceTokens(raw) {
			if seen[token] {
				continue
			}
			seen[token] = true
			ordered = append(ordered, token)
		}
	}

	add(primary)

	switch v := args["resources"].(type) {
	case string:
		add(v)
	case []string:
		for _, item := range v {
			add(item)
		}
	case []interface{}:
		for _, item := range v {
			add(fmt.Sprintf("%v", item))
		}
	}
	return ordered
}

func splitResourceTokens(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	fields := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ';' || r == ' '
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		token := strings.ToLower(strings.TrimSpace(field))
		if token == "" {
			continue
		}
		out = append(out, token)
	}
	return out
}

func buildK8sCommand(action, resource, name, namespace, output, selector string, tailLines int) string {
	switch action {
	case "get", "list": // "list" is a common alias for "get"
		return buildK8sGet(resource, name, namespace, output, selector)
	case "describe":
		return buildK8sDescribe(resource, name, namespace)
	case "logs":
		return buildK8sLogs(name, namespace, tailLines)
	case "top":
		return buildK8sTop(resource, namespace)
	default:
		// Unknown action: treat as "get" so the query still runs.
		slog.Warn("k8s_query unknown action, falling back to get", "action", action)
		return buildK8sGet(resource, name, namespace, output, selector)
	}
}

func buildK8sGet(resource, name, namespace, output, selector string) string {
	if resource == "" {
		return ""
	}
	parts := []string{"kubectl", "get", resource}
	if name != "" {
		parts = append(parts, name)
	}
	// For list queries (no specific name), default to --all-namespaces so resources in
	// non-default namespaces are always visible. describe/logs use appendNamespace directly.
	if name == "" && (namespace == "" || namespace == "all") {
		parts = append(parts, "--all-namespaces")
	} else {
		parts = appendNamespace(parts, namespace)
	}
	if output == "" {
		output = "wide"
	}
	parts = append(parts, "-o", output)
	if selector != "" {
		parts = append(parts, "-l", selector)
	}
	return strings.Join(parts, " ")
}

func buildK8sDescribe(resource, name, namespace string) string {
	if resource == "" || name == "" {
		return ""
	}
	parts := []string{"kubectl", "describe", resource, name}
	parts = appendNamespace(parts, namespace)
	return strings.Join(parts, " ")
}

func buildK8sLogs(name, namespace string, tailLines int) string {
	if name == "" {
		return ""
	}
	parts := []string{"kubectl", "logs", name, fmt.Sprintf("--tail=%d", tailLines)}
	parts = appendNamespace(parts, namespace)
	return strings.Join(parts, " ")
}

func buildK8sTop(resource, namespace string) string {
	if resource == "" {
		resource = "pods"
	}
	parts := []string{"kubectl", "top", resource}
	parts = appendNamespace(parts, namespace)
	return strings.Join(parts, " ")
}

func appendNamespace(parts []string, namespace string) []string {
	if namespace == "all" {
		return append(parts, "--all-namespaces")
	}
	if namespace != "" {
		return append(parts, "-n", namespace)
	}
	return parts
}

func nsFlag(namespace string) string {
	if namespace == "all" {
		return " --all-namespaces"
	}
	if namespace != "" {
		return " -n " + namespace
	}
	return ""
}
