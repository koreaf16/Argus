package workspace

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseExecOutput(t *testing.T) {
	metaToken := "__ARG_M_test__"
	pwd := base64.StdEncoding.EncodeToString([]byte("/home/dev"))
	user := base64.StdEncoding.EncodeToString([]byte("ubuntu"))
	output := strings.Join([]string{
		"line 1",
		"line 2",
		metaToken + ":0:" + pwd + ":" + user,
	}, "\n")

	stdout, code, cwd, who, err := parseExecOutput(output, metaToken)
	if err != nil {
		t.Fatalf("parseExecOutput returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("code=%d, want 0", code)
	}
	if cwd != "/home/dev" {
		t.Fatalf("cwd=%q, want /home/dev", cwd)
	}
	if who != "ubuntu" {
		t.Fatalf("user=%q, want ubuntu", who)
	}
	if strings.TrimSpace(stdout) != "line 1\nline 2" {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestParseExecOutput_MissingMetadata(t *testing.T) {
	_, _, _, _, err := parseExecOutput("hello", "__ARG_M_missing__")
	if err == nil {
		t.Fatal("expected parseExecOutput to fail for missing metadata")
	}
}

func TestParseExecOutput_TrimsDecodedMetadata(t *testing.T) {
	metaToken := "__ARG_M_trim__"
	pwd := base64.StdEncoding.EncodeToString([]byte("/home/oracle\n"))
	user := base64.StdEncoding.EncodeToString([]byte("oracle\n"))
	output := strings.Join([]string{
		"ok",
		metaToken + ":0:" + pwd + ":" + user,
	}, "\n")

	_, _, cwd, who, err := parseExecOutput(output, metaToken)
	if err != nil {
		t.Fatalf("parseExecOutput returned error: %v", err)
	}
	if cwd != "/home/oracle" {
		t.Fatalf("cwd=%q, want /home/oracle", cwd)
	}
	if who != "oracle" {
		t.Fatalf("user=%q, want oracle", who)
	}
}
