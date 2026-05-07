package probes

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// DockerProbe detects running Docker containers.
type DockerProbe struct{}

func (p *DockerProbe) Name() string { return "docker" }

func (p *DockerProbe) ScriptFragment() string {
	return `command -v docker >/dev/null 2>&1 && docker ps -a --format '{"ID":"{{.ID}}","Name":"{{.Names}}","Image":"{{.Image}}","State":"{{.State}}","Ports":"{{.Ports}}","Command":"{{.Command}}"}' 2>/dev/null | head -30`
}

func (p *DockerProbe) PreferredTimeout() time.Duration { return 5 * time.Second }

func (p *DockerProbe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, p.ScriptFragment())
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

func (p *DockerProbe) Parse(stdout string) (Result, error) {
	var containers []DockerContainer
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var c DockerContainer
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		containers = append(containers, c)
	}
	return Result{Docker: containers}, nil
}
