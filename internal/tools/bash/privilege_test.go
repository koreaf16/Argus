package bash

import "testing"

func TestRewriteSudoTokens(t *testing.T) {
	const qpw = "'SecretPW'"
	want := func(s string) string { return s }

	cases := []struct {
		name string
		cmd  string
		out  string
	}{
		{
			name: "simple sudo at start",
			cmd:  "sudo ls /root",
			out:  `printf "%s\n" 'SecretPW' | sudo -S ls /root`,
		},
		{
			name: "sudo after &&",
			cmd:  "psql --version && sudo systemctl status postgresql-17",
			out:  `psql --version && printf "%s\n" 'SecretPW' | sudo -S systemctl status postgresql-17`,
		},
		{
			name: "sudo after semicolon",
			cmd:  "echo hello; sudo reboot",
			out:  `echo hello; printf "%s\n" 'SecretPW' | sudo -S reboot`,
		},
		{
			name: "sudo after pipe",
			cmd:  "cat /etc/passwd | sudo tee /etc/shadow",
			out:  `cat /etc/passwd | printf "%s\n" 'SecretPW' | sudo -S tee /etc/shadow`,
		},
		{
			name: "already -S after pipe — no double wrap",
			cmd:  "printf pw | sudo -S ls",
			out:  want("printf pw | sudo -S ls"),
		},
		{
			name: "bare -S without stdin pipe — force inject",
			cmd:  "sudo -S systemctl stop postgresql-18.service",
			out:  `printf "%s\n" 'SecretPW' | sudo -S systemctl stop postgresql-18.service`,
		},
		{
			name: "compound bare -S — force inject for both",
			cmd:  "sudo -S systemctl stop x && sudo -S systemctl disable x",
			out:  `printf "%s\n" 'SecretPW' | sudo -S systemctl stop x && printf "%s\n" 'SecretPW' | sudo -S systemctl disable x`,
		},
		{
			name: "sudo in single-quoted string — skip",
			cmd:  "echo 'use sudo for root' && ls",
			out:  want("echo 'use sudo for root' && ls"),
		},
		{
			name: "sudo in double-quoted string — skip",
			cmd:  `echo "run sudo carefully"`,
			out:  want(`echo "run sudo carefully"`),
		},
		{
			name: "no sudo — unchanged",
			cmd:  "ls -la /tmp && echo done",
			out:  want("ls -la /tmp && echo done"),
		},
		{
			name: "multiple sudo in compound command",
			cmd:  "sudo apt update && sudo apt upgrade -y",
			out:  `printf "%s\n" 'SecretPW' | sudo -S apt update && printf "%s\n" 'SecretPW' | sudo -S apt upgrade -y`,
		},
		{
			name: "sudo after newline",
			cmd:  "echo hello\nsudo rm -rf /tmp/test",
			out:  "echo hello\nprintf \"%s\\n\" 'SecretPW' | sudo -S rm -rf /tmp/test",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rewriteSudoTokens(tc.cmd, qpw)
			if got != tc.out {
				t.Errorf("\ninput:    %q\nexpected: %q\ngot:      %q", tc.cmd, tc.out, got)
			}
		})
	}
}
