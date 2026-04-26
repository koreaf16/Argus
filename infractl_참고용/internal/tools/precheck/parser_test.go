// Package precheck
// File: parser_test.go
// Description: ParseChecks 순수 함수 단위 테스트
// Responsibility: 다양한 shell 명령어 패턴에서 올바른 Checker 목록이 추출되는지 검증한다.

package precheck

import (
	"testing"
)

func TestParseChecks_UserExists(t *testing.T) {
	cases := []struct {
		cmd      string
		wantUser string
	}{
		{"su - oracle", "oracle"},
		{"su -l oracle -c sqlplus", "oracle"},
		{"su oracle", "oracle"},
		{"sudo -u oracle sqlplus", "oracle"},
		{"sudo --user=oracle bash", "oracle"},
		{"sudo --user oracle rman", "oracle"},
		{"runuser -l oracle -c 'id'", "oracle"},
		{"runuser -u oracle id", "oracle"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindUserExists && c.Subject() == tc.wantUser {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce UserExistsChecker for %q; got %v", tc.cmd, tc.wantUser, checks)
			}
		})
	}
}

func TestParseChecks_NoUserForRoot(t *testing.T) {
	// su root / sudo -u root should not generate a user check (root always exists).
	cmds := []string{"su - root", "sudo -u root bash", "sudo bash"}
	for _, cmd := range cmds {
		checks := ParseChecks(cmd)
		for _, c := range checks {
			if c.Kind() == KindUserExists && c.Subject() == "root" {
				t.Errorf("ParseChecks(%q) should not check for root user existence", cmd)
			}
		}
	}
}

func TestParseChecks_BinaryExists(t *testing.T) {
	cases := []struct {
		cmd        string
		wantBinary string
		wantSev    Severity
	}{
		{"sqlplus / as sysdba", "sqlplus", SeverityBlock},
		{"rman target /", "rman", SeverityBlock},
		{"mysqladmin status", "mysqladmin", SeverityBlock},
		{"psql -U postgres", "psql", SeverityBlock},
		{"mongosh --host localhost", "mongosh", SeverityBlock},
		{"lsnrctl status", "lsnrctl", SeverityWarn},
		{"python3 script.py", "python3", SeverityWarn},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindBinaryExists && c.Subject() == tc.wantBinary {
					if c.Severity() != tc.wantSev {
						t.Errorf("ParseChecks(%q): binary %q severity = %v, want %v", tc.cmd, tc.wantBinary, c.Severity(), tc.wantSev)
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce BinaryExistsChecker for %q; got %v", tc.cmd, tc.wantBinary, checks)
			}
		})
	}
}

func TestParseChecks_DirExists(t *testing.T) {
	cases := []struct {
		cmd     string
		wantDir string
	}{
		{"cd /home/oracle && sqlplus", "/home/oracle"},
		{"cd /opt/app/install", "/opt/app/install"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindDirExists && c.Subject() == tc.wantDir {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce DirExistsChecker for %q; got %v", tc.cmd, tc.wantDir, checks)
			}
		})
	}
}

func TestParseChecks_NoDuplicates(t *testing.T) {
	// Same user mentioned multiple times should yield only one checker.
	cmd := "su - oracle -c 'sudo -u oracle sqlplus'"
	checks := ParseChecks(cmd)
	count := 0
	for _, c := range checks {
		if c.Kind() == KindUserExists && c.Subject() == "oracle" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("ParseChecks(%q): expected 1 UserExistsChecker for oracle, got %d", cmd, count)
	}
}

func TestParseChecks_StandardBinarySkipped(t *testing.T) {
	// Standard POSIX binaries (ls, cat, grep, etc.) should NOT get a binary check.
	cmds := []string{"ls -la /home", "cat /etc/passwd", "grep -r foo /var/log"}
	for _, cmd := range cmds {
		checks := ParseChecks(cmd)
		for _, c := range checks {
			if c.Kind() == KindBinaryExists {
				t.Errorf("ParseChecks(%q) unexpectedly produced BinaryExistsChecker for %q", cmd, c.Subject())
			}
		}
	}
}

func TestParseChecks_FileExists(t *testing.T) {
	cases := []struct {
		cmd      string
		wantFile string
		wantSev  Severity
	}{
		{"mv /tmp/LINUX.X64_193000_db_home.zip /home/oracle/", "/tmp/LINUX.X64_193000_db_home.zip", SeverityBlock},
		{"cp /tmp/patch.zip /home/oracle/", "/tmp/patch.zip", SeverityBlock},
		{"mv -f /tmp/file.tar.gz /opt/", "/tmp/file.tar.gz", SeverityBlock},
		{"unzip /tmp/db_home.zip -d /home/oracle/", "/tmp/db_home.zip", SeverityBlock},
		{"tar -xzf /tmp/archive.tar.gz", "/tmp/archive.tar.gz", SeverityBlock},
		{"tar xzf /tmp/archive.tar.gz", "/tmp/archive.tar.gz", SeverityBlock},
		{"tar -xf /tmp/archive.tar", "/tmp/archive.tar", SeverityBlock},
		{"source /etc/oracle/env.sh", "/etc/oracle/env.sh", SeverityWarn},
		{". /etc/profile", "/etc/profile", SeverityWarn},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindFileExists && c.Subject() == tc.wantFile {
					if c.Severity() != tc.wantSev {
						t.Errorf("ParseChecks(%q): file %q severity = %v, want %v", tc.cmd, tc.wantFile, c.Severity(), tc.wantSev)
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce FileExistsChecker for %q; got %v", tc.cmd, tc.wantFile, checks)
			}
		})
	}
}

func TestParseChecks_FileExistsEnvVarSkipped(t *testing.T) {
	// $VAR-based paths cannot be resolved at parse time — must be skipped.
	cmds := []string{
		"mv $PATCH_DIR/file.zip /home/oracle/",
		"unzip $ORACLE_ZIP -d /opt/",
	}
	for _, cmd := range cmds {
		checks := ParseChecks(cmd)
		for _, c := range checks {
			if c.Kind() == KindFileExists {
				t.Errorf("ParseChecks(%q) unexpectedly produced FileExistsChecker for %q (env var path)", cmd, c.Subject())
			}
		}
	}
}

func TestParseChecks_TarCreateNoFileCheck(t *testing.T) {
	// tar -czf creates the archive — output path should NOT be checked for existence.
	cmd := "tar -czf /tmp/backup.tar.gz /home/oracle"
	checks := ParseChecks(cmd)
	for _, c := range checks {
		if c.Kind() == KindFileExists && c.Subject() == "/tmp/backup.tar.gz" {
			t.Errorf("ParseChecks(%q): should not check output archive %q for existence", cmd, c.Subject())
		}
	}
}

func TestParseChecks_DirWritable(t *testing.T) {
	cases := []struct {
		cmd     string
		wantDir string
	}{
		{"unzip /tmp/db_home.zip -d /app/oracle/product/19c/dbhome_1", "/app/oracle/product/19c/dbhome_1"},
		{"unzip -q /tmp/patch.zip -d /app/oracle/product/19c/dbhome_1", "/app/oracle/product/19c/dbhome_1"},
		{"tar -xzf /tmp/archive.tar.gz -C /app/oracle/product/19c/dbhome_1", "/app/oracle/product/19c/dbhome_1"},
		{"tar xzf /tmp/archive.tar.gz -C /opt/oracle", "/opt/oracle"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindDirWritable && c.Subject() == tc.wantDir {
					if c.Severity() != SeverityBlock {
						t.Errorf("ParseChecks(%q): DirWritable %q severity = %v, want Block", tc.cmd, tc.wantDir, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce DirWritableChecker for %q; got %v", tc.cmd, tc.wantDir, checks)
			}
		})
	}
}

func TestParseChecks_EnvVarPathSkipped(t *testing.T) {
	// $ORACLE_HOME/bin/sqlplus — env var prefix should not trigger dir check.
	cmd := "$ORACLE_HOME/bin/sqlplus / as sysdba"
	checks := ParseChecks(cmd)
	for _, c := range checks {
		if c.Kind() == KindDirExists {
			t.Errorf("ParseChecks(%q) unexpectedly produced DirExistsChecker for %q (env var path)", cmd, c.Subject())
		}
	}
}

// ── New checker tests ─────────────────────────────────────────────────────────

func TestParseChecks_CpMvDest(t *testing.T) {
	cases := []struct {
		cmd     string
		wantDir string
		wantSev Severity
	}{
		{"cp /tmp/file.zip /home/oracle/", "/home/oracle/", SeverityBlock},
		{"mv -f /tmp/a.zip /opt/app", "/opt", SeverityBlock},
		{"cp /src/patch.zip /app/oracle/patches/", "/app/oracle/patches/", SeverityBlock},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindDirWritable && c.Subject() == tc.wantDir {
					if c.Severity() != tc.wantSev {
						t.Errorf("ParseChecks(%q): DirWritable %q severity = %v, want %v", tc.cmd, tc.wantDir, c.Severity(), tc.wantSev)
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce DirWritableChecker for %q; got %v", tc.cmd, tc.wantDir, checks)
			}
		})
	}
}

func TestParseChecks_Mkdir(t *testing.T) {
	cases := []struct {
		cmd        string
		wantParent string
	}{
		{"mkdir /home/oracle/oradata", "/home/oracle"},
		{"mkdir -p /app/oracle/product/19c/dbhome_1", "/app/oracle/product/19c"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindDirWritable && c.Subject() == tc.wantParent {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): mkdir parent %q severity = %v, want Warn", tc.cmd, tc.wantParent, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce DirWritableChecker(Warn) for parent %q; got %v", tc.cmd, tc.wantParent, checks)
			}
		})
	}
}

func TestParseChecks_ChmodOwnership(t *testing.T) {
	cases := []struct {
		cmd        string
		wantTarget string
	}{
		{"chmod 755 /home/oracle/app", "/home/oracle/app"},
		{"chown oracle:oinstall /opt/oracle/product", "/opt/oracle/product"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindFileOwned && c.Subject() == tc.wantTarget {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): FileOwned %q severity = %v, want Warn", tc.cmd, tc.wantTarget, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce FileOwnedChecker for %q; got %v", tc.cmd, tc.wantTarget, checks)
			}
		})
	}
}

func TestParseChecks_EnvVar(t *testing.T) {
	cases := []struct {
		cmd     string
		wantVar string
	}{
		{"mkdir -p $ORACLE_BASE/oradata", "ORACLE_BASE"},
		{"ls $ORACLE_HOME/bin/sqlplus", "ORACLE_HOME"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindEnvVarSet && c.Subject() == tc.wantVar {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): EnvVarSet %q severity = %v, want Warn", tc.cmd, tc.wantVar, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce EnvVarSetChecker for %q; got %v", tc.cmd, tc.wantVar, checks)
			}
		})
	}
}

func TestParseChecks_EnvVarStandardSkipped(t *testing.T) {
	// Standard shell variables must NOT generate an EnvVarSet check.
	cmds := []string{
		"ls $PATH/bin",
		"cd $HOME/oracle",
		"echo $USER/data",
	}
	for _, cmd := range cmds {
		checks := ParseChecks(cmd)
		for _, c := range checks {
			if c.Kind() == KindEnvVarSet {
				t.Errorf("ParseChecks(%q) unexpectedly produced EnvVarSetChecker for standard var %q", cmd, c.Subject())
			}
		}
	}
}

func TestParseChecks_DiskSpace(t *testing.T) {
	cases := []struct {
		cmd     string
		wantDir string
	}{
		{"unzip /tmp/db_home.zip -d /app/oracle/product/19c/dbhome_1", "/app/oracle/product/19c/dbhome_1"},
		{"tar -xzf /tmp/archive.tar.gz -C /opt/oracle", "/opt/oracle"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindDiskSpace && c.Subject() == tc.wantDir {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): DiskSpace %q severity = %v, want Warn", tc.cmd, tc.wantDir, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce DiskSpaceChecker for %q; got %v", tc.cmd, tc.wantDir, checks)
			}
		})
	}
}

func TestParseChecks_PortFree(t *testing.T) {
	cases := []struct {
		cmd      string
		wantPort string
		wantSev  Severity
	}{
		{"lsnrctl start", "1521", SeverityBlock},
		{"lsnrctl start LISTENER", "1521", SeverityBlock},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindPortFree && c.Subject() == tc.wantPort {
					if c.Severity() != tc.wantSev {
						t.Errorf("ParseChecks(%q): PortFree %q severity = %v, want %v", tc.cmd, tc.wantPort, c.Severity(), tc.wantSev)
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce PortFreeChecker for port %q; got %v", tc.cmd, tc.wantPort, checks)
			}
		})
	}
}

func TestParseChecks_PortFreeNotTriggeredByLsnrctlStatus(t *testing.T) {
	// lsnrctl status should NOT trigger a port check.
	cmd := "lsnrctl status"
	checks := ParseChecks(cmd)
	for _, c := range checks {
		if c.Kind() == KindPortFree {
			t.Errorf("ParseChecks(%q) unexpectedly produced PortFreeChecker", cmd)
		}
	}
}

func TestParseChecks_FsType(t *testing.T) {
	cases := []struct {
		cmd     string
		wantDir string
	}{
		{"unzip /tmp/db_home.zip -d /app/oracle/product/19c/dbhome_1", "/app/oracle/product/19c/dbhome_1"},
		{"tar -xzf /tmp/archive.tar.gz -C /opt/oracle", "/opt/oracle"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindFsType && c.Subject() == tc.wantDir {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce FsTypeChecker for %q; got %v", tc.cmd, tc.wantDir, checks)
			}
		})
	}
}

func TestParseChecks_SudoAllowed(t *testing.T) {
	cases := []struct {
		cmd        string
		wantTarget string
	}{
		{"sudo -u oracle sqlplus / as sysdba", "oracle"},
		{"sudo --user=oracle bash", "oracle"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindSudoAllowed && c.Subject() == tc.wantTarget {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): SudoAllowed %q severity = %v, want Warn", tc.cmd, tc.wantTarget, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce SudoAllowedChecker for %q; got %v", tc.cmd, tc.wantTarget, checks)
			}
		})
	}
}

func TestParseChecks_SudoAllowedNotForRoot(t *testing.T) {
	// sudo -u root should not generate a SudoAllowed check.
	cmd := "sudo -u root bash"
	checks := ParseChecks(cmd)
	for _, c := range checks {
		if c.Kind() == KindSudoAllowed {
			t.Errorf("ParseChecks(%q) unexpectedly produced SudoAllowedChecker for root", cmd)
		}
	}
}

func TestParseChecks_RedirectDest(t *testing.T) {
	cases := []struct {
		cmd        string
		wantParent string
	}{
		{"echo 'hello' > /home/oracle/init.sql", "/home/oracle"},
		{"echo 'data' >> /var/log/oracle/install.log", "/var/log/oracle"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindDirWritable && c.Subject() == tc.wantParent {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): redirect parent %q severity = %v, want Warn", tc.cmd, tc.wantParent, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce DirWritableChecker(Warn) for parent %q; got %v", tc.cmd, tc.wantParent, checks)
			}
		})
	}
}

func TestParseChecks_TouchTarget(t *testing.T) {
	cases := []struct {
		cmd        string
		wantParent string
	}{
		{"touch /home/oracle/.bashrc", "/home/oracle"},
		{"touch /opt/oracle/oradata/lock.pid", "/opt/oracle/oradata"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindDirWritable && c.Subject() == tc.wantParent {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): touch parent %q severity = %v, want Warn", tc.cmd, tc.wantParent, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce DirWritableChecker(Warn) for parent %q; got %v", tc.cmd, tc.wantParent, checks)
			}
		})
	}
}

// ── New checker tests (Phase 2) ───────────────────────────────────────────────

func TestParseChecks_RmDangerous(t *testing.T) {
	// Commands that MUST produce an RmSafetyChecker.
	dangerous := []string{
		"rm -rf /",
		"rm -rf /*",
		"rm -fR ~/",
		"rm -rf $HOME/",
		"rm -rf $HOME/data",
	}
	for _, cmd := range dangerous {
		t.Run(cmd, func(t *testing.T) {
			checks := ParseChecks(cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindRmSafety {
					if c.Severity() != SeverityBlock {
						t.Errorf("ParseChecks(%q): RmSafety severity = %v, want Block", cmd, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce RmSafetyChecker; got %v", cmd, checks)
			}
		})
	}

	// Commands that must NOT produce RmSafetyChecker.
	safe := []string{
		"rm -f /tmp/file.txt",
		"rm -rf /home/oracle/patch",
		"rm /tmp/old.zip",
	}
	for _, cmd := range safe {
		t.Run("safe:"+cmd, func(t *testing.T) {
			checks := ParseChecks(cmd)
			for _, c := range checks {
				if c.Kind() == KindRmSafety {
					t.Errorf("ParseChecks(%q) unexpectedly produced RmSafetyChecker", cmd)
				}
			}
		})
	}
}

func TestParseChecks_TeeTarget(t *testing.T) {
	cases := []struct {
		cmd        string
		wantParent string
	}{
		{"echo hello | tee /home/oracle/init.sql", "/home/oracle"},
		{"cat /tmp/data | tee /var/log/oracle/out.log", "/var/log/oracle"},
		{"tee /opt/oracle/conf/listener.ora", "/opt/oracle/conf"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindDirWritable && c.Subject() == tc.wantParent {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): tee parent %q severity = %v, want Warn", tc.cmd, tc.wantParent, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce DirWritableChecker(Warn) for parent %q; got %v", tc.cmd, tc.wantParent, checks)
			}
		})
	}
}

func TestParseChecks_Systemctl(t *testing.T) {
	cases := []struct {
		cmd        string
		wantSvc    string
		wantAction string
	}{
		{"systemctl start oracle", "oracle", "start"},
		{"systemctl restart pmon", "pmon", "restart"},
		{"systemctl stop oracle-xe", "oracle-xe", "stop"},
		{"systemctl enable oracle.service", "oracle.service", "enable"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindServiceExists && c.Subject() == tc.wantSvc {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): ServiceExists %q severity = %v, want Warn", tc.cmd, tc.wantSvc, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce ServiceExistsChecker for %q; got %v", tc.cmd, tc.wantSvc, checks)
			}
		})
	}
}

func TestParseChecks_CurlWget(t *testing.T) {
	cases := []struct {
		cmd      string
		wantHost string
		wantSev  Severity
	}{
		{"curl https://example.com/file.zip", "example.com:443", SeverityWarn},
		{"wget http://repo.example.com:8080/pkg.tar.gz", "repo.example.com:8080", SeverityWarn},
		{"curl -L https://download.oracle.com/otn/linux/oracle19c.zip", "download.oracle.com:443", SeverityWarn},
		{"wget ftp://ftp.oracle.com/patch.zip", "ftp.oracle.com:21", SeverityWarn},
	}

	for _, tc := range cases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindHostReachable && c.Subject() == tc.wantHost {
					if c.Severity() != tc.wantSev {
						t.Errorf("ParseChecks(%q): HostReachable %q severity = %v, want %v", tc.cmd, tc.wantHost, c.Severity(), tc.wantSev)
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce HostReachableChecker for %q; got %v", tc.cmd, tc.wantHost, checks)
			}
		})
	}
}

func TestParseChecks_NotRoot(t *testing.T) {
	// Binaries that MUST produce NotRootChecker.
	rootBins := []struct {
		cmd    string
		binary string
	}{
		{"sqlplus / as sysdba", "sqlplus"},
		{"dbca -silent -createDatabase", "dbca"},
		{"lsnrctl start", "lsnrctl"},
		{"rman target /", "rman"},
		{"asmcmd ls", "asmcmd"},
	}
	for _, tc := range rootBins {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindNotRoot && c.Subject() == tc.binary {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): NotRoot %q severity = %v, want Warn", tc.cmd, tc.binary, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce NotRootChecker for %q; got %v", tc.cmd, tc.binary, checks)
			}
		})
	}

	// Standard commands must NOT produce NotRootChecker.
	safe := []string{"ls /home", "cat /etc/passwd", "grep foo /var/log/syslog"}
	for _, cmd := range safe {
		t.Run("safe:"+cmd, func(t *testing.T) {
			checks := ParseChecks(cmd)
			for _, c := range checks {
				if c.Kind() == KindNotRoot {
					t.Errorf("ParseChecks(%q) unexpectedly produced NotRootChecker for %q", cmd, c.Subject())
				}
			}
		})
	}
}

func TestParseChecks_Kill(t *testing.T) {
	pidCases := []struct {
		cmd     string
		wantPID string
	}{
		{"kill -9 1234", "1234"},
		{"kill 5678", "5678"},
		{"kill -SIGTERM 42", "42"},
	}
	for _, tc := range pidCases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindProcessExists && c.Subject() == tc.wantPID {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): ProcessExists %q severity = %v, want Warn", tc.cmd, tc.wantPID, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce ProcessExistsChecker for PID %q; got %v", tc.cmd, tc.wantPID, checks)
			}
		})
	}

	nameCases := []struct {
		cmd      string
		wantName string
	}{
		{"pkill pmon", "pmon"},
		{"killall oracle", "oracle"},
		{"pkill -9 smon", "smon"},
	}
	for _, tc := range nameCases {
		t.Run(tc.cmd, func(t *testing.T) {
			checks := ParseChecks(tc.cmd)
			found := false
			for _, c := range checks {
				if c.Kind() == KindProcessExists && c.Subject() == tc.wantName {
					if c.Severity() != SeverityWarn {
						t.Errorf("ParseChecks(%q): ProcessExists %q severity = %v, want Warn", tc.cmd, tc.wantName, c.Severity())
					}
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ParseChecks(%q) did not produce ProcessExistsChecker(name) for %q; got %v", tc.cmd, tc.wantName, checks)
			}
		})
	}
}
