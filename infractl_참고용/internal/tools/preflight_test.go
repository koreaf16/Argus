// Package tools
// File: preflight_test.go
// Description: preflight 순수 함수 유닛 테스트
// Responsibility: CheckCriticalPath, ScanCommand, parseDFAvailableBytes의 동작을 검증한다.

package tools

import (
	"testing"
)

// ── CheckCriticalPath ────────────────────────────────────────────────────────

func TestCheckCriticalPath_BlockedPaths(t *testing.T) {
	blocked := []string{
		"/proc",
		"/proc/self/mem",
		"/sys/kernel",
		"/dev/sda",
	}
	for _, p := range blocked {
		warning, isCritical := CheckCriticalPath(p)
		if !isCritical {
			t.Errorf("path %q: expected isCritical=true, got false (warning=%q)", p, warning)
		}
		if warning == "" {
			t.Errorf("path %q: expected non-empty warning", p)
		}
	}
}

func TestCheckCriticalPath_WarnPaths(t *testing.T) {
	warned := []string{
		"/etc/nginx/nginx.conf",
		"/etc",
		"/boot/grub/grub.cfg",
		"/usr/bin/bash",
		"/sbin/init",
		"/lib/systemd/systemd",
		"/var/run/docker.sock",
	}
	for _, p := range warned {
		warning, isCritical := CheckCriticalPath(p)
		if isCritical {
			t.Errorf("path %q: expected isCritical=false (warn-level), got true", p)
		}
		if warning == "" {
			t.Errorf("path %q: expected non-empty warning", p)
		}
	}
}

func TestCheckCriticalPath_SafePaths(t *testing.T) {
	safe := []string{
		"/home/sandbox/oracle_install",
		"/tmp/work",
		"/opt/myapp",
		"/u01/app/oracle",
		"/var/log/nginx",
		"relative/path",
		"",
	}
	for _, p := range safe {
		warning, isCritical := CheckCriticalPath(p)
		if warning != "" || isCritical {
			t.Errorf("path %q: expected no warning/critical, got warning=%q isCritical=%v", p, warning, isCritical)
		}
	}
}

func TestCheckCriticalPath_DevNullAllowed(t *testing.T) {
	// /dev/null, /dev/zero, /dev/std* are safe discard devices commonly used in
	// shell redirections (e.g. 2>/dev/null) and must not be blocked.
	allowed := []string{
		"/dev/null",
		"/dev/zero",
		"/dev/stdin",
		"/dev/stdout",
		"/dev/stderr",
	}
	for _, p := range allowed {
		warning, isCritical := CheckCriticalPath(p)
		if warning != "" || isCritical {
			t.Errorf("path %q: expected no warning/critical (null device), got warning=%q isCritical=%v", p, warning, isCritical)
		}
	}
}

func TestCheckCriticalPath_DevBlockDevicesStillBlocked(t *testing.T) {
	// Non-null /dev entries (block/char devices) must remain blocked.
	blocked := []string{
		"/dev/sda",
		"/dev/sdb1",
		"/dev/tty",
	}
	for _, p := range blocked {
		warning, isCritical := CheckCriticalPath(p)
		if !isCritical {
			t.Errorf("path %q: expected isCritical=true, got false (warning=%q)", p, warning)
		}
	}
}

func TestCheckCriticalPath_PrefixBoundary(t *testing.T) {
	// /etcfoo must NOT match /etc
	warning, isCritical := CheckCriticalPath("/etcfoo/bar")
	if warning != "" || isCritical {
		t.Errorf("/etcfoo/bar should not match /etc prefix, got warning=%q", warning)
	}
	// /etc itself must match
	warning2, _ := CheckCriticalPath("/etc")
	if warning2 == "" {
		t.Error("/etc should produce a warning")
	}
}

// ── ScanCommand ──────────────────────────────────────────────────────────────

func TestScanCommand_HighRisk(t *testing.T) {
	cases := []struct {
		cmd         string
		wantDesc    string
		wantPathsIn []string
	}{
		{
			cmd:         "rm -rf /tmp/oracle_install",
			wantDesc:    "recursive force delete",
			wantPathsIn: []string{"/tmp/oracle_install"},
		},
		{
			cmd:      "dd if=/dev/zero of=/dev/sda bs=4M",
			wantDesc: "raw disk write",
		},
		{
			cmd:      "mkfs.ext4 /dev/sdb1",
			wantDesc: "filesystem creation",
		},
		{
			cmd:      "fdisk /dev/sdc",
			wantDesc: "disk partition tool",
		},
	}
	for _, tc := range cases {
		r := ScanCommand(tc.cmd)
		if !r.IsDiskModifying {
			t.Errorf("cmd=%q: expected IsDiskModifying=true", tc.cmd)
		}
		if r.Risk != RiskHigh {
			t.Errorf("cmd=%q: expected RiskHigh, got %q", tc.cmd, r.Risk)
		}
		if tc.wantDesc != "" && !containsSubstr(r.Description, tc.wantDesc) {
			t.Errorf("cmd=%q: description %q should contain %q", tc.cmd, r.Description, tc.wantDesc)
		}
		for _, path := range tc.wantPathsIn {
			if !containsPath(r.TargetPaths, path) {
				t.Errorf("cmd=%q: TargetPaths %v should contain %q", tc.cmd, r.TargetPaths, path)
			}
		}
	}
}

func TestScanCommand_MediumRisk(t *testing.T) {
	cases := []struct {
		cmd string
	}{
		{"tar -xzf archive.tar.gz -C /opt/app"},
		{"unzip package.zip /opt/pkg"},
		{"cp -r src/ /var/www/html/"},
		{"mv /tmp/file.conf /etc/app/file.conf"},
		{"yum install -y nginx"},
		{"dnf install oracle-database-ee-19c"},
		{"apt-get install -y postgresql"},
		{"wget -O /tmp/installer.sh https://example.com/installer.sh"},
		{"curl -o /opt/agent.tar.gz https://example.com/agent.tar.gz"},
		{"mkdir -p /home/sandbox/oracle_install"},
	}
	for _, tc := range cases {
		r := ScanCommand(tc.cmd)
		if !r.IsDiskModifying {
			t.Errorf("cmd=%q: expected IsDiskModifying=true", tc.cmd)
		}
		if r.Risk != RiskMedium {
			t.Errorf("cmd=%q: expected RiskMedium, got %q", tc.cmd, r.Risk)
		}
	}
}

func TestScanCommand_LowRisk(t *testing.T) {
	cases := []string{
		"chmod 755 /opt/app/bin/start.sh",
		"chown oracle:oinstall /u01/app/oracle",
		"pip install requests",
	}
	for _, cmd := range cases {
		r := ScanCommand(cmd)
		if !r.IsDiskModifying {
			t.Errorf("cmd=%q: expected IsDiskModifying=true", cmd)
		}
		if r.Risk != RiskLow {
			t.Errorf("cmd=%q: expected RiskLow, got %q", cmd, r.Risk)
		}
	}
}

func TestScanCommand_ReadOnly(t *testing.T) {
	readOnly := []string{
		"ls -la /home/sandbox",
		"cat /etc/os-release",
		"df -h",
		"ps aux",
		"systemctl status nginx",
		"echo hello",
	}
	for _, cmd := range readOnly {
		r := ScanCommand(cmd)
		if r.IsDiskModifying {
			t.Errorf("cmd=%q: expected IsDiskModifying=false, got true (desc=%q)", cmd, r.Description)
		}
	}
}

func TestScanCommand_RedirectCapture(t *testing.T) {
	r := ScanCommand("echo 'hello' > /tmp/test.txt")
	if !r.IsDiskModifying {
		t.Error("redirect > should be detected as disk-modifying")
	}
	if !containsPath(r.TargetPaths, "/tmp/test.txt") {
		t.Errorf("expected /tmp/test.txt in TargetPaths, got %v", r.TargetPaths)
	}
}

func TestScanCommand_PackageInstallDoesNotTreatPackageNameAsPath(t *testing.T) {
	r := ScanCommand("sudo dnf install -y binutils gcc gcc-c++ make sysstat unzip libaio ksh")
	if !r.IsDiskModifying {
		t.Fatal("expected package install to be disk-modifying")
	}
	if r.Risk != RiskMedium {
		t.Fatalf("expected medium risk, got %q", r.Risk)
	}
	if containsPath(r.TargetPaths, "ksh") {
		t.Fatalf("package name should not be parsed as target path: %v", r.TargetPaths)
	}
	if !containsPath(r.TargetPaths, "/") {
		t.Fatalf("expected root filesystem target path: %v", r.TargetPaths)
	}
	if !containsPath(r.TargetPaths, "/var") {
		t.Fatalf("expected /var target path for package manager writes: %v", r.TargetPaths)
	}
}

func TestScanCommand_TeeOptionExtractsRealPath(t *testing.T) {
	cmd := "cat << 'EOF' | sudo tee -a /etc/bash.bashrc << 'EOD'"
	r := ScanCommand(cmd)
	if !r.IsDiskModifying {
		t.Fatal("expected tee write to be disk-modifying")
	}
	if !containsPath(r.TargetPaths, "/etc/bash.bashrc") {
		t.Fatalf("expected tee target path in result: %v", r.TargetPaths)
	}
	if containsPath(r.TargetPaths, "-a") {
		t.Fatalf("tee option must not be parsed as a path: %v", r.TargetPaths)
	}
}

// ── parseDFAvailableBytes ────────────────────────────────────────────────────

func TestParseDFAvailableBytes(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantBytes int64
		wantErr   bool
	}{
		{
			name: "normal output",
			input: `Filesystem     1K-blocks      Used Available Use% Mounted on
/dev/sda1       41943040  10485760  31457280  25% /`,
			wantBytes: 31457280 * 1024,
		},
		{
			name: "long device name continuation",
			input: `Filesystem
     1K-blocks      Used Available Use% Mounted on
/dev/mapper/rootvg-rootlv
                  41943040  10485760  20971520  34% /`,
			wantBytes: 20971520 * 1024,
		},
		{
			name:    "empty output",
			input:   "",
			wantErr: true,
		},
		{
			name:    "only header",
			input:   "Filesystem  1K-blocks  Used  Available  Use%  Mounted-on",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseDFAvailableBytes(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (result=%d)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantBytes {
				t.Errorf("got %d bytes, want %d", got, tc.wantBytes)
			}
		})
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func containsSubstr(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && (s == sub || len(s) >= len(sub) &&
		(s[:len(sub)] == sub || containsSubstr(s[1:], sub)))
}

func containsPath(paths []string, target string) bool {
	for _, p := range paths {
		if p == target {
			return true
		}
	}
	return false
}
