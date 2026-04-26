package readonly

import "testing"

func TestIsReadOnly_posix(t *testing.T) {
	tests := []struct {
		argv []string
		want bool
	}{
		{[]string{"ls", "-la"}, true},
		{[]string{"cat", "file.txt"}, true},
		{[]string{"grep", "-r", "pattern", "."}, true},
		{[]string{"head", "-n", "10", "file"}, true},
		{[]string{"wc", "-l", "file"}, true},
		{[]string{"echo", "hello"}, true},
		{[]string{"pwd"}, true},
		{[]string{"find", ".", "-name", "*.go", "-print"}, true},
		{[]string{"find", ".", "-name", "*.go", "-delete"}, false}, // 쓰기 금지
		{[]string{"sed", "-i", "s/a/b/", "file"}, false},           // -i 금지
		{[]string{"rm", "-rf", "/"}, false},                        // 미등록 → false
	}
	for _, tt := range tests {
		got := IsReadOnly(tt.argv)
		if got != tt.want {
			t.Errorf("IsReadOnly(%v) = %v, want %v", tt.argv, got, tt.want)
		}
	}
}

func TestIsReadOnly_git(t *testing.T) {
	tests := []struct {
		argv []string
		want bool
	}{
		{[]string{"git", "log", "--oneline", "-n", "10"}, true},
		{[]string{"git", "status", "-s"}, true},
		{[]string{"git", "diff", "--stat"}, true},
		{[]string{"git", "blame", "file.go"}, true},
		{[]string{"git", "branch", "-a"}, true},
		{[]string{"git", "rev-parse", "--abbrev-ref", "HEAD"}, true},
		{[]string{"git", "config", "--list"}, true},
		{[]string{"git", "reflog", "expire"}, false}, // 위험 서브커맨드
		{[]string{"git", "push"}, false},             // 미등록
		{[]string{"git", "commit", "-m", "msg"}, false},
	}
	for _, tt := range tests {
		got := IsReadOnly(tt.argv)
		if got != tt.want {
			t.Errorf("IsReadOnly(%v) = %v, want %v", tt.argv, got, tt.want)
		}
	}
}

func TestIsReadOnly_docker(t *testing.T) {
	tests := []struct {
		argv []string
		want bool
	}{
		{[]string{"docker", "ps", "-a"}, true},
		{[]string{"docker", "images", "--format", "{{.Repository}}"}, true},
		{[]string{"docker", "logs", "-f", "container"}, true},
		{[]string{"docker", "inspect", "container"}, true},
		{[]string{"docker", "run", "image"}, false}, // 미등록
		{[]string{"docker", "rm", "container"}, false},
	}
	for _, tt := range tests {
		got := IsReadOnly(tt.argv)
		if got != tt.want {
			t.Errorf("IsReadOnly(%v) = %v, want %v", tt.argv, got, tt.want)
		}
	}
}

func TestIsReadOnly_rg(t *testing.T) {
	if !IsReadOnly([]string{"rg", "pattern", "."}) {
		t.Error("rg pattern . should be read-only")
	}
	if !IsReadOnly([]string{"rg", "-i", "-n", "pattern"}) {
		t.Error("rg with safe flags should be read-only")
	}
}

func TestIsReadOnly_empty(t *testing.T) {
	if IsReadOnly(nil) {
		t.Error("nil argv should not be read-only")
	}
	if IsReadOnly([]string{}) {
		t.Error("empty argv should not be read-only")
	}
}

func TestContainsVulnerableUncPath(t *testing.T) {
	// UNC 검사는 Windows 에서만 활성화되므로 단순 호출만 검증
	_ = ContainsVulnerableUncPath(`\\server\share`)
	_ = ContainsVulnerableUncPath(`//server/share`)
	_ = ContainsVulnerableUncPath(`/usr/bin/ls`)
}
