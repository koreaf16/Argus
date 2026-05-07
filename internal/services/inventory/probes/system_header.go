package probes

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SystemHeaderResult holds basic system identity data collected by SystemHeaderProbe.
type SystemHeaderResult struct {
	OS            string // trimmed uname -srm  (e.g. "Linux host 5.14.0")
	OSPretty      string // /etc/os-release PRETTY_NAME (e.g. "Red Hat Enterprise Linux 9.4")
	User          string // whoami output
	Shell         string // $SHELL value
	Uptime        string // trimmed uptime string (e.g. "31 days, 13:43")
	Memory        string // first data line of free -h
	Disk          string // tail line of df -h /
	ListenerCount int    // listening TCP port count
	ServiceCount  int    // running systemd service count
}

const systemHeaderScript = `set +e
printf '<<HDR:os>>\n'
uname -srm 2>/dev/null
printf '<<HDR:pretty>>\n'
grep '^PRETTY_NAME=' /etc/os-release 2>/dev/null | cut -d= -f2- | tr -d '"'
printf '<<HDR:user>>\n'
whoami 2>/dev/null
printf '<<HDR:shell>>\n'
echo "$SHELL"
printf '<<HDR:uptime>>\n'
uptime 2>/dev/null
printf '<<HDR:mem>>\n'
free -h 2>/dev/null | head -2
printf '<<HDR:disk>>\n'
df -h / 2>/dev/null | tail -1
printf '<<HDR:listen>>\n'
ss -tln 2>/dev/null | grep -c LISTEN || echo 0
printf '<<HDR:svc>>\n'
systemctl list-units --type=service --state=running --no-legend 2>/dev/null | wc -l || echo 0
printf '<<HDR:end>>\n'`

// SystemHeaderProbe collects basic system identity using unprivileged commands (~2s).
type SystemHeaderProbe struct{}

func (p *SystemHeaderProbe) Name() string { return "system_header" }

func (p *SystemHeaderProbe) ScriptFragment() string { return systemHeaderScript }

func (p *SystemHeaderProbe) Parse(stdout string) (Result, error) {
	return Result{SystemHeader: parseSystemHeader(stdout)}, nil
}

func (p *SystemHeaderProbe) PreferredTimeout() time.Duration { return 3 * time.Second }

func (p *SystemHeaderProbe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, systemHeaderScript)
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

// RunDirect executes the probe directly with a 3s timeout and returns the parsed result.
func (p *SystemHeaderProbe) RunDirect(ctx context.Context, fn ProbeExec) (*SystemHeaderResult, error) {
	out, err := runScript(ctx, fn, 3*time.Second, systemHeaderScript)
	if err != nil {
		return nil, err
	}
	return parseSystemHeader(out), nil
}

func parseSystemHeader(stdout string) *SystemHeaderResult {
	info := &SystemHeaderResult{}
	sections := splitHDRSections(stdout)

	rawOS := firstNonEmptyLine(sections["os"])
	info.OS = trimOSString(rawOS)
	info.OSPretty = firstNonEmptyLine(sections["pretty"])
	info.User = firstNonEmptyLine(sections["user"])
	info.Shell = firstNonEmptyLine(sections["shell"])
	info.Uptime = trimUptimeString(firstNonEmptyLine(sections["uptime"]))
	info.Memory = firstNonEmptyLine(sections["mem"])
	info.Disk = firstNonEmptyLine(sections["disk"])

	fmt.Sscanf(strings.TrimSpace(sections["listen"]), "%d", &info.ListenerCount)
	fmt.Sscanf(strings.TrimSpace(sections["svc"]), "%d", &info.ServiceCount)
	return info
}

func splitHDRSections(raw string) map[string]string {
	sections := make(map[string]string)
	var current string
	var buf strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "<<HDR:") && strings.HasSuffix(line, ">>") {
			if current != "" && current != "end" {
				sections[current] = buf.String()
				buf.Reset()
			}
			current = line[6 : len(line)-2]
			continue
		}
		if current != "" && current != "end" {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	if current != "" && current != "end" && buf.Len() > 0 {
		sections[current] = buf.String()
	}
	return sections
}

// trimOSString shortens a uname -srm line: "Linux host 5.14.0-611.el9.x86_64" → "Linux host 5.14.0"
func trimOSString(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return line
	}
	kernel := fields[2]
	if dash := strings.IndexByte(kernel, '-'); dash >= 0 {
		kernel = kernel[:dash]
	}
	return fields[0] + " " + fields[1] + " " + kernel
}

// trimUptimeString extracts the duration from uptime output.
// "10:03:26 up 31 days, 13:43,  2 users,  load average: 1.35" → "31 days, 13:43"
func trimUptimeString(s string) string {
	s = strings.TrimSpace(s)
	upIdx := strings.Index(s, " up ")
	if upIdx < 0 {
		return s
	}
	rest := strings.TrimSpace(s[upIdx+4:])
	if idx := strings.Index(rest, " user"); idx >= 0 {
		if comma := strings.LastIndex(rest[:idx], ","); comma >= 0 {
			rest = strings.TrimSpace(rest[:comma])
		} else {
			rest = strings.TrimSpace(rest[:idx])
		}
	}
	return rest
}

