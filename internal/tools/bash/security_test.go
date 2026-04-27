package bash

import (
	"strings"
	"testing"
)

func TestCheckBashSecurity(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		wantOk  bool
		wantMsg string // 포함 여부 검사
	}{
		// --- Layer 2: 에디터 차단 ---
		{name: "vim 차단", cmd: "vim test.txt", wantOk: false, wantMsg: "vim"},
		{name: "vi 차단", cmd: "vi /etc/hosts", wantOk: false, wantMsg: "vi"},
		{name: "nvim 차단", cmd: "nvim file.go", wantOk: false, wantMsg: "nvim"},
		{name: "nano 차단", cmd: "nano /tmp/test", wantOk: false, wantMsg: "nano"},
		{name: "emacs 차단", cmd: "emacs foo.txt", wantOk: false, wantMsg: "emacs"},

		// --- 페이저 차단 ---
		{name: "less 차단", cmd: "less file.log", wantOk: false, wantMsg: "less"},
		{name: "more 차단", cmd: "more /var/log/syslog", wantOk: false, wantMsg: "more"},
		{name: "man 차단", cmd: "man kubectl", wantOk: false, wantMsg: "man"},

		// --- 모니터 차단 ---
		{name: "top 차단", cmd: "top", wantOk: false, wantMsg: "top"},
		{name: "htop 차단", cmd: "htop", wantOk: false, wantMsg: "htop"},
		{name: "tmux 차단", cmd: "tmux new-session", wantOk: false, wantMsg: "tmux"},

		// --- 멀티워드 패턴 차단 ---
		{name: "kubectl edit 차단", cmd: "kubectl edit deployment X -n dbs", wantOk: false, wantMsg: "kubectl edit"},
		{name: "git rebase -i 차단", cmd: "git rebase -i HEAD~3", wantOk: false, wantMsg: "git rebase"},
		{name: "git rebase --interactive 차단", cmd: "git rebase --interactive HEAD~5", wantOk: false, wantMsg: "git rebase"},
		{name: "git add -i 차단", cmd: "git add -i", wantOk: false, wantMsg: "git add"},
		{name: "git add -p 차단", cmd: "git add -p", wantOk: false, wantMsg: "git add"},
		{name: "git stash -p 차단", cmd: "git stash -p", wantOk: false, wantMsg: "git stash"},
		{name: "crontab -e 차단", cmd: "crontab -e", wantOk: false, wantMsg: "crontab"},
		{name: "visudo 차단", cmd: "visudo", wantOk: false, wantMsg: "visudo"},
		{name: "kubectl exec -it 차단", cmd: "kubectl exec -it pod-name -- bash", wantOk: false, wantMsg: "kubectl"},
		{name: "docker exec -it 차단", cmd: "docker exec -it container bash", wantOk: false, wantMsg: "docker"},

		// --- 파이프라인 내 페이저 차단 ---
		{name: "cat | less 차단", cmd: "cat file.log | less", wantOk: false, wantMsg: "less"},
		{name: "git log | more 차단", cmd: "git log --oneline | more", wantOk: false, wantMsg: "more"},

		// --- REPL 차단/허용 ---
		{name: "python 인자없음 차단", cmd: "python", wantOk: false, wantMsg: "python"},
		{name: "python3 인자없음 차단", cmd: "python3", wantOk: false, wantMsg: "python3"},
		{name: "node 인자없음 차단", cmd: "node", wantOk: false, wantMsg: "node"},
		{name: "psql 인자없음 차단", cmd: "psql", wantOk: false, wantMsg: "psql"},
		{name: "python -i 차단", cmd: "python -i script.py", wantOk: false, wantMsg: "python"},

		// --- 정상 통과 ---
		{name: "python script 통과", cmd: "python script.py --flag value", wantOk: true},
		{name: "python3 -c 통과", cmd: `python3 -c "print(1)"`, wantOk: true},
		{name: "node -e 통과", cmd: `node -e "console.log(1)"`, wantOk: true},
		{name: "psql -c 통과", cmd: `psql -c "SELECT 1"`, wantOk: true},
		{name: "git commit -m 통과", cmd: `git commit -m "fix: something"`, wantOk: true},
		{name: "git log 통과", cmd: "git log --oneline -10", wantOk: true},
		{name: "git add 파일 통과", cmd: "git add internal/tools/bash/security.go", wantOk: true},
		{name: "kubectl get 통과", cmd: "kubectl get deployment -n dbs", wantOk: true},
		{name: "kubectl describe 통과", cmd: "kubectl describe pod X", wantOk: true},
		{name: "kubectl exec 비대화형 통과", cmd: "kubectl exec pod-name -- ls /tmp", wantOk: true},
		{name: "sudo apt 통과", cmd: "sudo apt update", wantOk: true},
		{name: "cat file 통과", cmd: "cat /etc/hosts", wantOk: true},
		{name: "tail -n 통과", cmd: "tail -n 100 /var/log/syslog", wantOk: true},
		{name: "ssh 비TTY 통과", cmd: "ssh user@host ls -la", wantOk: true},
		{name: "빈 명령 통과", cmd: "", wantOk: true},
		{name: "env 오버라이드 crontab -e 차단", cmd: "EDITOR=vi crontab -e", wantOk: false, wantMsg: "crontab"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := CheckBashSecurity(tt.cmd)
			if ok != tt.wantOk {
				t.Errorf("CheckBashSecurity(%q) ok=%v, want %v (msg: %s)", tt.cmd, ok, tt.wantOk, msg)
			}
			if !tt.wantOk && tt.wantMsg != "" {
				if !strings.Contains(strings.ToLower(msg), strings.ToLower(tt.wantMsg)) {
					t.Errorf("CheckBashSecurity(%q) msg=%q, want to contain %q", tt.cmd, msg, tt.wantMsg)
				}
			}
		})
	}
}

func TestSplitCommandPipeline(t *testing.T) {
	tests := []struct {
		input    string
		wantLen  int
	}{
		{"cat file | less", 2},
		{"cmd1; cmd2", 2},
		{"cmd1 && cmd2", 2},
		{"single", 1},
		{"a | b | c", 3},
	}
	for _, tt := range tests {
		got := splitCommandPipeline(tt.input)
		if len(got) != tt.wantLen {
			t.Errorf("splitCommandPipeline(%q) = %d segments, want %d", tt.input, len(got), tt.wantLen)
		}
	}
}

func TestIsReplWithArgs(t *testing.T) {
	tests := []struct {
		fields []string
		want   bool
	}{
		{[]string{"python"}, false},
		{[]string{"python", "script.py"}, true},
		{[]string{"python", "-i", "script.py"}, false},
		{[]string{"python3", "-c", "print(1)"}, true},
		{[]string{"node"}, false},
		{[]string{"node", "-e", "console.log(1)"}, true},
	}
	for _, tt := range tests {
		got := isReplWithArgs(tt.fields[0], tt.fields)
		if got != tt.want {
			t.Errorf("isReplWithArgs(%v) = %v, want %v", tt.fields, got, tt.want)
		}
	}
}
