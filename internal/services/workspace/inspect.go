package workspace

import (
	"context"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// RunInspect collects environment information for the given workspace alias
// and stores the result in the inspect cache.
func (m *Manager) RunInspect(ctx context.Context, alias string) (InspectSnapshot, error) {
	alias = m.ResolveAlias(alias)
	snap := InspectSnapshot{
		Alias:       alias,
		CollectedAt: time.Now(),
		Errors:      make(map[string]string),
	}

	if alias == LocalAlias {
		snap = m.runLocalInspect(ctx, snap)
	} else {
		snap = m.runRemoteInspect(ctx, alias, snap)
	}

	if len(snap.Errors) == 0 {
		snap.Errors = nil
	}
	m.SetInspectSnapshot(alias, snap)
	return snap, nil
}

func (m *Manager) runLocalInspect(ctx context.Context, snap InspectSnapshot) InspectSnapshot {
	snap.OS = runtime.GOOS + "/" + runtime.GOARCH

	if s := os.Getenv("SHELL"); s != "" {
		snap.Shell = s
	} else if runtime.GOOS == "windows" {
		snap.Shell = "cmd.exe / powershell (Git Bash recommended)"
	}

	if user := os.Getenv("USERNAME"); user != "" {
		snap.User = user
	} else if user := os.Getenv("USER"); user != "" {
		snap.User = user
	}

	if cwd, err := os.Getwd(); err == nil {
		snap.CWD = cwd
	}

	remoteCmds := map[string]string{
		"uptime":    "uptime 2>/dev/null",
		"memory":    "free -h 2>/dev/null",
		"disk":      "df -h 2>/dev/null | head -10",
		"processes": "ps -eo pid,user,pcpu,pmem,comm --sort=-pcpu 2>/dev/null | head -15",
		"docker":    "docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}' 2>/dev/null | head -10",
	}
	m.fillFromCommands(ctx, LocalAlias, remoteCmds, &snap)
	return snap
}

func (m *Manager) runRemoteInspect(ctx context.Context, alias string, snap InspectSnapshot) InspectSnapshot {
	cmds := map[string]string{
		"os":        "{ uname -a; echo '---'; cat /etc/os-release 2>/dev/null | grep -E '^(NAME|VERSION)='; } 2>/dev/null",
		"shell":     "echo $SHELL",
		"uptime":    "uptime 2>/dev/null",
		"user_cwd":  "whoami && pwd",
		"memory":    "free -h 2>/dev/null || vm_stat 2>/dev/null | head -6",
		"disk":      "df -h 2>/dev/null | head -10",
		"listeners": "ss -tulpn 2>/dev/null | grep LISTEN | head -25 || netstat -tulpn 2>/dev/null | grep LISTEN | head -25",
		"services":  "systemctl list-units --type=service --state=running --no-pager --plain 2>/dev/null | head -30 || service --status-all 2>/dev/null | grep + 2>/dev/null | head -20",
		"processes": "ps -eo pid,user,pcpu,pmem,comm --sort=-pcpu 2>/dev/null | head -20",
		"docker":    "docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}' 2>/dev/null | head -15",
	}
	m.fillFromCommands(ctx, alias, cmds, &snap)
	return snap
}

func (m *Manager) fillFromCommands(ctx context.Context, alias string, cmds map[string]string, snap *InspectSnapshot) {
	var wg sync.WaitGroup
	var mu sync.Mutex

	for key, cmd := range cmds {
		wg.Add(1)
		go func(field, command string) {
			defer wg.Done()
			res, err := m.ExecWithOptions(ctx, alias, command, ExecOptions{}, nil)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				snap.Errors[field] = err.Error()
				return
			}
			out := strings.TrimSpace(res.Stdout)
			if out == "" {
				return
			}
			switch field {
			case "os":
				snap.OS = out
			case "shell":
				snap.Shell = out
			case "uptime":
				snap.Uptime = out
			case "user_cwd":
				lines := strings.SplitN(out, "\n", 2)
				if len(lines) >= 1 {
					snap.User = strings.TrimSpace(lines[0])
				}
				if len(lines) >= 2 {
					snap.CWD = strings.TrimSpace(lines[1])
				}
			case "memory":
				snap.Memory = out
			case "disk":
				snap.Disk = out
			case "listeners":
				snap.Listeners = out
			case "services":
				snap.Services = out
			case "processes":
				snap.Processes = out
			case "docker":
				snap.Docker = out
			}
		}(key, cmd)
	}
	wg.Wait()
}
