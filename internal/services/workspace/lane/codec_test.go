package lane

import (
	"strings"
	"testing"
)

func TestCodecWrapShape(t *testing.T) {
	c := &Codec{Nonce: "abc123"}
	out := c.Wrap(`psql -c "SELECT now();"`)

	if !strings.Contains(out, `psql -c "SELECT now();"`) {
		t.Fatalf("user command not preserved verbatim:\n%s", out)
	}
	if !strings.Contains(out, SentinelPrefix+"abc123:") {
		t.Fatalf("sentinel prefix missing:\n%s", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("wrap must end with newline:\n%q", out)
	}
}

func TestCodecRoundtrip(t *testing.T) {
	c := &Codec{Nonce: "deadbeef"}
	stdout := "PostgreSQL 18.3 on x86_64-pc-linux-gnu\n"
	sentinel := SentinelPrefix + c.Nonce + ":0:/var/lib/pgsql:postgres\n"
	stream := []byte(stdout + sentinel + "trailing")

	res, ok := c.Parse(stream)
	if !ok {
		t.Fatal("Parse returned false on complete sentinel")
	}
	if res.Stdout != strings.TrimRight(stdout, "\n") {
		t.Errorf("stdout = %q, want %q", res.Stdout, strings.TrimRight(stdout, "\n"))
	}
	if res.Code != 0 {
		t.Errorf("code = %d, want 0", res.Code)
	}
	if res.CWD != "/var/lib/pgsql" {
		t.Errorf("cwd = %q", res.CWD)
	}
	if res.User != "postgres" {
		t.Errorf("user = %q", res.User)
	}
	if res.BytesUsed != len(stdout)+len(sentinel) {
		t.Errorf("bytesUsed = %d, want %d", res.BytesUsed, len(stdout)+len(sentinel))
	}
}

func TestCodecPartialStream(t *testing.T) {
	c := &Codec{Nonce: "n"}
	if _, ok := c.Parse([]byte("some output without sentinel")); ok {
		t.Fatal("Parse should not match before sentinel arrives")
	}
	if _, ok := c.Parse([]byte(SentinelPrefix + "n:0:/tmp:user")); ok {
		t.Fatal("Parse must wait for terminating newline")
	}
}

func TestCodecNonceMismatch(t *testing.T) {
	c := &Codec{Nonce: "mine"}
	stream := []byte("hello\n" + SentinelPrefix + "yours:0:/tmp:user\n")
	if _, ok := c.Parse(stream); ok {
		t.Fatal("Parse must ignore other-nonce sentinels")
	}
}

func TestCodecNonzeroExit(t *testing.T) {
	c := &Codec{Nonce: "x"}
	stream := []byte("oops: bad cmd\n" + SentinelPrefix + "x:127:/home/u:u\n")
	res, ok := c.Parse(stream)
	if !ok {
		t.Fatal("Parse failed on nonzero exit")
	}
	if res.Code != 127 {
		t.Errorf("code = %d, want 127", res.Code)
	}
}
