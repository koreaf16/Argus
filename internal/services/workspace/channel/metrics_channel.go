package channel

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

// metricsChannel collects /proc-style snapshots on demand. It does NOT poll
// in the background — Collect runs a single batched script when the caller
// asks for a fresh snapshot. The script's output is split by sentinel markers
// that cannot collide with the user's lane sentinel.
type metricsChannel struct {
	key    ChannelKey
	client *ssh.Client

	mu       sync.Mutex
	state    ChannelState
	lastUsed time.Time
	closed   atomic.Bool
}

// metricsScript is the single shell script the metrics channel runs to gather
// every numeric snapshot in one round-trip. Each section is delimited by an
// ASCII sentinel of the form ===<KEY>=== so the parser can split robustly
// regardless of locale or quoting in the underlying tool output.
const metricsScript = `set +e
echo '===LOADAVG==='
cat /proc/loadavg 2>/dev/null
echo '===MEMINFO==='
free -b 2>/dev/null
echo '===DISK==='
df -B1 -P 2>/dev/null
echo '===UPTIME==='
cat /proc/uptime 2>/dev/null
echo '===PROCESSES==='
ps -eo pid,user,pcpu,pmem,comm --sort=-pcpu 2>/dev/null | head -20
echo '===GPU==='
( command -v nvidia-smi >/dev/null 2>&1 && nvidia-smi --query-gpu=name,utilization.gpu,memory.used,memory.total --format=csv,noheader 2>/dev/null ) || echo ""
echo '===END==='
`

// metricsSection is one parsed block from metricsScript output.
type metricsSection struct {
	Key  string
	Body string
}

func openMetricsChannel(client *ssh.Client, alias string) (*metricsChannel, error) {
	if client == nil {
		return nil, fmt.Errorf("channel: metrics requires a live ssh client")
	}
	return &metricsChannel{
		key: ChannelKey{
			Alias:     alias,
			Privilege: PrivilegeDefault,
			Purpose:   PurposeMetrics,
		},
		client:   client,
		state:    StateReady,
		lastUsed: time.Now(),
	}, nil
}

func (c *metricsChannel) Key() ChannelKey { return c.key }

func (c *metricsChannel) Snapshot() ChannelSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return ChannelSnapshot{
		Key:      c.key,
		State:    c.state,
		LastUsed: c.lastUsed,
	}
}

// Collect runs metricsScript in a fresh ssh.Session. It does NOT touch the
// caller's exec channel, so concurrent user commands are not serialized
// behind metrics collection.
func (c *metricsChannel) Collect(ctx context.Context) (RawMetrics, error) {
	if c.closed.Load() {
		return RawMetrics{}, fmt.Errorf("channel: metrics closed")
	}

	c.mu.Lock()
	c.state = StateBusy
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.state = StateReady
		c.lastUsed = time.Now()
		c.mu.Unlock()
	}()

	session, err := c.client.NewSession()
	if err != nil {
		return RawMetrics{}, fmt.Errorf("metrics: open session: %w", err)
	}
	defer session.Close()

	var out bytes.Buffer
	var stderrBuf bytes.Buffer
	session.Stdout = &out
	session.Stderr = &stderrBuf

	done := make(chan error, 1)
	go func() {
		done <- session.Run(metricsScript)
	}()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGTERM)
		<-done
		return RawMetrics{}, ctx.Err()
	case err := <-done:
		if err != nil {
			// ssh.ExitError is fine if some sub-commands failed (e.g. nvidia-smi
			// missing). The script itself uses `set +e` so this should not fire,
			// but stay defensive.
			return parseMetrics(out.String(), stderrBuf.String()), nil
		}
		return parseMetrics(out.String(), stderrBuf.String()), nil
	}
}

func (c *metricsChannel) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.mu.Lock()
	c.state = StateClosed
	c.mu.Unlock()
	return nil
}

// parseMetrics splits the batched script output into RawMetrics fields.
// Unknown sections are ignored. Errors observed on stderr are stored in
// the per-tool error map keyed by section.
func parseMetrics(stdout, stderr string) RawMetrics {
	sections := splitMetricsSections(stdout)
	out := RawMetrics{Errors: map[string]string{}}
	for _, sec := range sections {
		body := strings.TrimSpace(sec.Body)
		switch sec.Key {
		case "LOADAVG":
			out.LoadAvg = body
		case "MEMINFO":
			out.MemInfo = body
		case "DISK":
			out.DiskInfo = body
		case "UPTIME":
			out.UptimeRaw = body
		case "PROCESSES":
			out.Processes = body
		case "GPU":
			out.GPU = body
		}
	}
	if strings.TrimSpace(stderr) != "" {
		out.Errors["stderr"] = strings.TrimSpace(stderr)
	}
	return out
}

func splitMetricsSections(stdout string) []metricsSection {
	const marker = "==="
	lines := strings.Split(stdout, "\n")
	var sections []metricsSection
	var current *metricsSection
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		if strings.HasPrefix(line, marker) && strings.HasSuffix(line, marker) {
			key := strings.TrimSuffix(strings.TrimPrefix(line, marker), marker)
			if key == "END" {
				if current != nil {
					sections = append(sections, *current)
					current = nil
				}
				break
			}
			if current != nil {
				sections = append(sections, *current)
			}
			current = &metricsSection{Key: key}
			continue
		}
		if current != nil {
			if current.Body != "" {
				current.Body += "\n"
			}
			current.Body += line
		}
	}
	if current != nil {
		sections = append(sections, *current)
	}
	return sections
}
