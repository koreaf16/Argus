package servercopy

import (
	"testing"

	"github.com/koreaf16/argus/internal/services/workspace"
)

func TestParseCopyRequestEndpoints(t *testing.T) {
	srcAlias, srcPath, dstAlias, dstPath, err := parseCopyRequest(
		"local:C:\\Users\\jhkwa\\Downloads\\a.zip",
		"",
		"",
		"dev:/tmp/a.zip",
		"",
		"",
		workspace.LocalAlias,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srcAlias != "local" || srcPath != "C:\\Users\\jhkwa\\Downloads\\a.zip" {
		t.Fatalf("unexpected source: %s:%s", srcAlias, srcPath)
	}
	if dstAlias != "dev" || dstPath != "/tmp/a.zip" {
		t.Fatalf("unexpected destination: %s:%s", dstAlias, dstPath)
	}
}

func TestParseCopyRequestLegacyFields(t *testing.T) {
	srcAlias, srcPath, dstAlias, dstPath, err := parseCopyRequest(
		"",
		"local",
		"a.txt",
		"",
		"dev",
		"/tmp/a.txt",
		workspace.LocalAlias,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if srcAlias != "local" || srcPath != "a.txt" || dstAlias != "dev" || dstPath != "/tmp/a.txt" {
		t.Fatalf("unexpected parsed values: %s %s %s %s", srcAlias, srcPath, dstAlias, dstPath)
	}
}
