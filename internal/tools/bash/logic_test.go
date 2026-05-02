package bash

import (
	"testing"
)

func TestParseTargetUser(t *testing.T) {
	tests := []struct {
		cmd      string
		wantUser string
		wantBody string
	}{
		{"su - oracle -c \"ls -al\"", "oracle", "ls -al"},
		{"su oracle -c 'ls'", "oracle", "ls"},
		{"su - -c \"whoami\"", "root", "whoami"},
		{"sudo -u oracle id", "oracle", "id"},
		{"sudo ls /root", "root", "ls /root"},
		{"ls -al", "", ""},
		// compound sudo su — the real target user should be extracted
		{`sudo su - oracle -c "sqlplus -s / as sysdba"`, "oracle", "sqlplus -s / as sysdba"},
		{`sudo su oracle -c "whoami"`, "oracle", "whoami"},
		{`sudo su - postgres -c "psql -l"`, "postgres", "psql -l"},
		{`sudo su - -c "id"`, "root", "id"},
	}

	for _, tt := range tests {
		user, body := parseTargetUser(tt.cmd)
		if user != tt.wantUser || body != tt.wantBody {
			t.Errorf("parseTargetUser(%q) = (%q, %q), want (%q, %q)", tt.cmd, user, body, tt.wantUser, tt.wantBody)
		}
	}
}

// Smart Rewriter 로직은 Call 메서드 내부에 있어 직접 테스트하기 어려우므로
// parseTargetUser가 정확하면 핵심 논리는 검증된 것으로 간주합니다.

func TestStripRedundantPrivilegeWrapper(t *testing.T) {
	tests := []struct {
		name        string
		command     string
		currentUser string
		want        string
	}{
		{
			name:        "strip su wrapper when target equals current user",
			command:     `su - oracle -c "sqlplus / as sysdba"`,
			currentUser: "oracle",
			want:        "sqlplus / as sysdba",
		},
		{
			name:        "strip sudo wrapper when target equals current user",
			command:     "sudo -u oracle id",
			currentUser: "oracle",
			want:        "id",
		},
		{
			name:        "keep su wrapper for different target user",
			command:     `su - root -c "id"`,
			currentUser: "oracle",
			want:        `su - root -c "id"`,
		},
		{
			name:        "keep su command without -c body",
			command:     "su - oracle",
			currentUser: "oracle",
			want:        "su - oracle",
		},
	}

	for _, tt := range tests {
		got := stripRedundantPrivilegeWrapper(tt.command, tt.currentUser)
		if got != tt.want {
			t.Errorf("%s: stripRedundantPrivilegeWrapper(%q, %q) = %q, want %q", tt.name, tt.command, tt.currentUser, got, tt.want)
		}
	}
}
