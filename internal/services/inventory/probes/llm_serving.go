package probes

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LLMServingProbe detects vLLM, Ollama, SGLang, and TGI processes.
type LLMServingProbe struct{}

func (p *LLMServingProbe) Name() string { return "llm_serving" }

func (p *LLMServingProbe) ScriptFragment() string {
	return `
set +e
# 1. Process scan
for engine_pat in 'vllm\.serve\|python.*-m vllm\|vllm\.entrypoints' 'sglang\.launch_server\|python.*-m sglang' 'text-generation-launcher\|text-generation-server'; do
  pgrep -fa "$engine_pat" 2>/dev/null | while IFS= read -r pline; do
    echo "PROC_VLLM:$pline"
  done
done
pgrep -fa 'ollama' 2>/dev/null | while IFS= read -r pline; do
  echo "PROC_OLLAMA:$pline"
done
# 2. Port listeners on common LLM ports
ss -tlnp 2>/dev/null | grep -E ':(8000|8001|8002|8003|8080|11434) ' | while IFS= read -r pline; do
  echo "PORT:$pline"
done
# 3. Ollama model list
command -v ollama >/dev/null 2>&1 && ollama list 2>/dev/null | tail -n +2 | while IFS= read -r pline; do
  echo "OLLAMA_MODEL:$pline"
done
# 4. vLLM /v1/models for native/docker (skip if no listener)
for port in 8000 8001 8002 8003 8080; do
  ss -tlnp 2>/dev/null | grep -q ":${port} " && \
    curl -s -m 3 "http://localhost:${port}/v1/models" 2>/dev/null | grep -o '"id":"[^"]*"' | while IFS= read -r pline; do
      echo "VLLM_MODEL:${port}:${pline}"
    done
done
# 5. cgroup for hosted-by detection (vllm/sglang pids)
for pid in $(pgrep -f 'vllm\|sglang\|text-generation' 2>/dev/null | head -5); do
  echo "CGROUP:${pid}:$(cat /proc/${pid}/cgroup 2>/dev/null | head -3 | tr '\n' '|')"
done
`
}

func (p *LLMServingProbe) PreferredTimeout() time.Duration { return 6 * time.Second }

func (p *LLMServingProbe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, p.ScriptFragment())
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

func (p *LLMServingProbe) Parse(stdout string) (Result, error) {
	type engineEntry struct {
		engine  string
		port    int
		models  []string
		cgroup  string
		cgpid   int
	}

	byPort := map[int]*engineEntry{}
	var ollamaModels []string
	cgroupByPid := map[int]string{}

	for _, raw := range strings.Split(stdout, "\n") {
		line := strings.TrimRight(raw, "\r")
		switch {
		case strings.HasPrefix(line, "PROC_VLLM:"):
			// Just note that vllm is running; port will be discovered via PORT:
			_ = strings.TrimPrefix(line, "PROC_VLLM:")

		case strings.HasPrefix(line, "PROC_OLLAMA:"):
			_ = strings.TrimPrefix(line, "PROC_OLLAMA:")

		case strings.HasPrefix(line, "PORT:"):
			// e.g. "LISTEN 0 128 0.0.0.0:8000 0.0.0.0:* users:(("python3",pid=1234,fd=5))"
			rest := strings.TrimPrefix(line, "PORT:")
			port := extractPort(rest)
			if port <= 0 {
				continue
			}
			if _, ok := byPort[port]; !ok {
				engine := engineForPort(port)
				byPort[port] = &engineEntry{engine: engine, port: port}
			}

		case strings.HasPrefix(line, "OLLAMA_MODEL:"):
			// "llama3:8b 2024-... 4.7 GB ..."
			rest := strings.TrimPrefix(line, "OLLAMA_MODEL:")
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				ollamaModels = append(ollamaModels, fields[0])
			}

		case strings.HasPrefix(line, "VLLM_MODEL:"):
			// "VLLM_MODEL:8000:"id":"google/gemma-2-27b-it""
			rest := strings.TrimPrefix(line, "VLLM_MODEL:")
			parts := strings.SplitN(rest, ":", 2)
			if len(parts) != 2 {
				continue
			}
			port, _ := strconv.Atoi(parts[0])
			modelID := strings.TrimPrefix(parts[1], `"id":"`)
			modelID = strings.TrimSuffix(modelID, `"`)
			if port > 0 && modelID != "" {
				if _, ok := byPort[port]; !ok {
					byPort[port] = &engineEntry{engine: "vllm", port: port}
				}
				byPort[port].engine = "vllm"
				byPort[port].models = append(byPort[port].models, modelID)
			}

		case strings.HasPrefix(line, "CGROUP:"):
			// "CGROUP:1234:12:devices:/kubepods/..."
			rest := strings.TrimPrefix(line, "CGROUP:")
			parts := strings.SplitN(rest, ":", 2)
			if len(parts) != 2 {
				continue
			}
			pid, _ := strconv.Atoi(parts[0])
			if pid > 0 {
				cgroupByPid[pid] = parts[1]
			}
		}
	}

	// Apply cgroup → hosting
	var results []LLMServingResult
	for _, e := range byPort {
		res := LLMServingResult{
			Engine: e.engine,
			Port:   e.port,
			Models: e.models,
		}
		// Find cgroup for any pid listening on this port
		for pid, cg := range cgroupByPid {
			res.CgroupPid = pid
			res.Cgroup = cg
			break
		}
		results = append(results, res)
	}

	// Ollama is always native (no separate port in byPort if port 11434)
	if len(ollamaModels) > 0 {
		if e, ok := byPort[11434]; ok {
			e.engine = "ollama"
			e.models = ollamaModels
		} else {
			results = append(results, LLMServingResult{
				Engine: "ollama",
				Port:   11434,
				Models: ollamaModels,
			})
		}
	}

	return Result{LLMServing: results}, nil
}

func extractPort(ssLine string) int {
	// Look for patterns like "0.0.0.0:8000" or ":::8000"
	for _, field := range strings.Fields(ssLine) {
		if idx := strings.LastIndex(field, ":"); idx >= 0 {
			portStr := field[idx+1:]
			port, err := strconv.Atoi(portStr)
			if err == nil && port > 0 {
				return port
			}
		}
	}
	return 0
}

func engineForPort(port int) string {
	switch port {
	case 11434:
		return "ollama"
	case 8000, 8001, 8002, 8003, 8080:
		return "vllm"
	}
	return fmt.Sprintf("unknown(%d)", port)
}
