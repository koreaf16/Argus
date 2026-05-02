package channel

import (
	"strings"
	"testing"
)

func TestParseMetricsSplitsSections(t *testing.T) {
	stdout := strings.Join([]string{
		"===LOADAVG===",
		"0.10 0.20 0.30 1/200 12345",
		"===MEMINFO===",
		"               total        used        free",
		"Mem:        16777216     8388608     8388608",
		"===DISK===",
		"Filesystem 1B-blocks Used Available Capacity Mounted on",
		"/dev/sda1  100000000 50000 49000     1%      /",
		"===UPTIME===",
		"123456.78 98765.43",
		"===PROCESSES===",
		"  PID USER     %CPU %MEM COMMAND",
		"    1 root      0.0  0.1 systemd",
		"===GPU===",
		"NVIDIA T4, 25 %, 1024 MiB, 16384 MiB",
		"===END===",
	}, "\n")
	got := parseMetrics(stdout, "")

	if !strings.Contains(got.LoadAvg, "0.10 0.20 0.30") {
		t.Errorf("LoadAvg missing payload: %q", got.LoadAvg)
	}
	if !strings.Contains(got.MemInfo, "Mem:") {
		t.Errorf("MemInfo missing: %q", got.MemInfo)
	}
	if !strings.Contains(got.DiskInfo, "/dev/sda1") {
		t.Errorf("DiskInfo missing: %q", got.DiskInfo)
	}
	if !strings.Contains(got.UptimeRaw, "123456.78") {
		t.Errorf("UptimeRaw missing: %q", got.UptimeRaw)
	}
	if !strings.Contains(got.Processes, "systemd") {
		t.Errorf("Processes missing: %q", got.Processes)
	}
	if !strings.Contains(got.GPU, "NVIDIA T4") {
		t.Errorf("GPU missing: %q", got.GPU)
	}
}

func TestParseMetricsIgnoresUnknownSections(t *testing.T) {
	stdout := "===WHATEVER===\nignored\n===END==="
	got := parseMetrics(stdout, "")
	if got.LoadAvg != "" || got.MemInfo != "" {
		t.Errorf("unknown section leaked into result: %+v", got)
	}
}

func TestParseMetricsCarriesStderr(t *testing.T) {
	got := parseMetrics("===END===", "permission denied")
	if got.Errors["stderr"] != "permission denied" {
		t.Errorf("expected stderr to flow into Errors, got %v", got.Errors)
	}
}

func TestSplitMetricsSectionsTerminatesOnEnd(t *testing.T) {
	stdout := "===A===\nbody\n===END===\nignored after end"
	secs := splitMetricsSections(stdout)
	if len(secs) != 1 || secs[0].Key != "A" || secs[0].Body != "body" {
		t.Fatalf("unexpected sections: %+v", secs)
	}
}
