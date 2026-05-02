package workspace

import (
	"context"
	"fmt"
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
	// 원격 inspect는 lane(단일 PTY)이 명령을 직렬화하므로 10개 병렬 ExecWithOptions
	// 가 사실상 직렬 실행되어 20~30초까지 늘어진다. 한 번의 batched 스크립트로 묶어
	// 모든 inspect 명령을 단일 lane 호출로 끝낸다.
	// 분리자는 순수 ASCII만 사용한다. 과거 0x1E(RS) 바이트를 쓰면 PTY를 통과하는
	// 동안 bash readline 등에 변형되어 inspect Exec이 멎는 사례가 있었다.
	const sep = "<<ARGUS_INSPECT:%s>>"
	fields := []struct {
		key string
		cmd string
	}{
		{"os", "{ uname -a; echo '---'; cat /etc/os-release 2>/dev/null | grep -E '^(NAME|VERSION)='; } 2>/dev/null"},
		{"shell", "echo $SHELL"},
		{"uptime", "uptime 2>/dev/null"},
		{"user_cwd", "whoami && pwd"},
		{"memory", "free -h 2>/dev/null || vm_stat 2>/dev/null | head -6"},
		{"disk", "df -h 2>/dev/null | head -10"},
		{"listeners", "ss -tulpn 2>/dev/null | grep LISTEN | head -25 || netstat -tulpn 2>/dev/null | grep LISTEN | head -25"},
		{"services", "systemctl list-units --type=service --state=running --no-pager --plain 2>/dev/null | head -30 || service --status-all 2>/dev/null | grep + 2>/dev/null | head -20"},
		{"processes", "ps -eo pid,user,pcpu,pmem,comm --sort=-pcpu 2>/dev/null | head -20"},
		// command -v 가드는 PackageKit의 command_not_found_handle 같은 대화형
		// 설치 프롬프트가 (이미 LaneInitScript에서 unset 되었더라도) lane을
		// 멎게 하지 못하도록 하는 이중 안전장치다.
		{"docker", "command -v docker >/dev/null 2>&1 && docker ps --format '{{.Names}}\t{{.Image}}\t{{.Status}}' 2>/dev/null | head -15"},
	}

	var script strings.Builder
	for _, f := range fields {
		script.WriteString("printf '")
		script.WriteString(strings.ReplaceAll(fmt.Sprintf(sep, f.key), "'", "'\\''"))
		script.WriteString("\\n'\n")
		script.WriteString(f.cmd)
		script.WriteString("\n")
	}
	script.WriteString("printf '")
	script.WriteString(strings.ReplaceAll(fmt.Sprintf(sep, "END"), "'", "'\\''"))
	script.WriteString("\\n'\n")

	res, err := m.ExecWithOptions(ctx, alias, script.String(), ExecOptions{Shell: "bash"}, nil)
	if err != nil {
		snap.Errors["inspect"] = err.Error()
		return snap
	}

	out := res.Stdout
	for i, f := range fields {
		startMarker := fmt.Sprintf(sep, f.key)
		startIdx := strings.Index(out, startMarker)
		if startIdx < 0 {
			snap.Errors[f.key] = "section marker missing"
			continue
		}
		startIdx += len(startMarker)
		// 다음 marker 까지 잘라낸다.
		var endMarker string
		if i+1 < len(fields) {
			endMarker = fmt.Sprintf(sep, fields[i+1].key)
		} else {
			endMarker = fmt.Sprintf(sep, "END")
		}
		endIdx := strings.Index(out[startIdx:], endMarker)
		if endIdx < 0 {
			endIdx = len(out) - startIdx
		}
		section := strings.TrimSpace(out[startIdx : startIdx+endIdx])
		if section == "" {
			continue
		}
		switch f.key {
		case "os":
			snap.OS = section
		case "shell":
			snap.Shell = section
		case "uptime":
			snap.Uptime = section
		case "user_cwd":
			lines := strings.SplitN(section, "\n", 2)
			if len(lines) >= 1 {
				snap.User = strings.TrimSpace(lines[0])
			}
			if len(lines) >= 2 {
				snap.CWD = strings.TrimSpace(lines[1])
			}
		case "memory":
			snap.Memory = section
		case "disk":
			snap.Disk = section
		case "listeners":
			snap.Listeners = section
		case "services":
			snap.Services = section
		case "processes":
			snap.Processes = section
		case "docker":
			snap.Docker = section
		}
	}
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
