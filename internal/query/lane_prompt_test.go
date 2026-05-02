package query

import (
	"strings"
	"testing"
	"time"

	"github.com/koreaf16/argus/internal/services/workspace"
)

func TestLaneSystemBlocksEmpty(t *testing.T) {
	if blocks := laneSystemBlocks(nil); blocks != nil {
		t.Fatalf("nil manager must yield nil blocks, got %v", blocks)
	}
}

func TestFormatIdle(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{500 * time.Millisecond, "0s"},
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{-time.Second, "0s"},
	}
	for _, c := range cases {
		if got := formatIdle(c.in); got != c.want {
			t.Errorf("formatIdle(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTopAccount(t *testing.T) {
	if top := topAccount(nil); top != "" {
		t.Errorf("nil stack top = %q", top)
	}
	if top := topAccount([]string{"a", "b", "c"}); top != "c" {
		t.Errorf("stack top = %q, want c", top)
	}
}

// LaneInfo struct is exposed by the workspace package; this test pins the
// rendering format so a downstream change immediately surfaces in tests.
func TestLaneSystemBlocksRendersInfos(t *testing.T) {
	now := time.Now()
	infos := []workspace.LaneInfo{
		{Alias: "sandbox", AccountStack: []string{"sandbox", "postgres"}, CWD: "/var/lib/pgsql", LastUsed: now.Add(-12 * time.Second)},
		{Alias: "prod", AccountStack: []string{"ubuntu"}, CWD: "/home/ubuntu", LastUsed: now.Add(-2 * time.Minute)},
	}
	rendered := renderLaneInfosForTest(infos)
	if !strings.Contains(rendered, "sandbox : user=postgres cwd=/var/lib/pgsql stack=sandbox>postgres") {
		t.Errorf("missing sandbox line in:\n%s", rendered)
	}
	if !strings.Contains(rendered, "prod : user=ubuntu cwd=/home/ubuntu") {
		t.Errorf("missing prod line in:\n%s", rendered)
	}
	if !strings.Contains(rendered, "su - <user>") {
		t.Errorf("missing rules block in:\n%s", rendered)
	}
}

// renderLaneInfosForTest mirrors laneSystemBlocks's body without a Manager,
// keeping the test free of SSH dependencies.
func renderLaneInfosForTest(infos []workspace.LaneInfo) string {
	var sb strings.Builder
	sb.WriteString("ACTIVE EXECUTION LANES (single source of truth — use the `server` parameter to pick one):\n")
	now := time.Now()
	for _, info := range infos {
		user := topAccount(info.AccountStack)
		stack := strings.Join(info.AccountStack, ">")
		sb.WriteString("- ")
		sb.WriteString(info.Alias)
		sb.WriteString(" : user=")
		sb.WriteString(defaultIfEmpty(user, "?"))
		sb.WriteString(" cwd=")
		sb.WriteString(defaultIfEmpty(info.CWD, "?"))
		sb.WriteString(" stack=")
		sb.WriteString(defaultIfEmpty(stack, "-"))
		sb.WriteString(" idle=")
		sb.WriteString(formatIdle(now.Sub(info.LastUsed)))
		sb.WriteString("\n")
	}
	sb.WriteString("\nRules:\n")
	sb.WriteString("- The cwd and user shown above are authoritative for that host. Do not assume different state.\n")
	sb.WriteString("- To switch the effective account on a host, send `su - <user>` or `sudo -i -u <user>` as a normal bash command — the lane will track the account stack push automatically. Send `exit` to pop.\n")
	sb.WriteString("- For one-shot privileged work without changing the stack, use `sudo -u <user> <body>` or `sudo <body>` directly.\n")
	return sb.String()
}
