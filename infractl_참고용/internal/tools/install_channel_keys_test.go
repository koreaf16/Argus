package tools

import (
	"strings"
	"testing"
)

func TestAppendManagedAuthorizedKeyPreservesExistingKeys(t *testing.T) {
	existing := "ssh-ed25519 AAAA user1@host\nssh-rsa BBBB user2@host\n"

	updated, added, err := AppendManagedAuthorizedKey(existing, "ssh-ed25519 CCCC temp@runner", "task-123")
	if err != nil {
		t.Fatalf("AppendManagedAuthorizedKey() error = %v", err)
	}
	if !added {
		t.Fatal("expected added=true")
	}
	if !strings.Contains(updated, "ssh-ed25519 AAAA user1@host") {
		t.Fatalf("existing key was removed: %q", updated)
	}
	if !strings.Contains(updated, "ssh-rsa BBBB user2@host") {
		t.Fatalf("existing key was removed: %q", updated)
	}
	if !strings.Contains(updated, "infractl-ephemeral:task-123") {
		t.Fatalf("managed marker missing: %q", updated)
	}
}

func TestAppendManagedAuthorizedKeyNoDuplicateForSameMarker(t *testing.T) {
	existing := "ssh-ed25519 AAAA user1@host infractl-ephemeral:task-123\n"

	updated, added, err := AppendManagedAuthorizedKey(existing, "ssh-ed25519 CCCC temp@runner", "task-123")
	if err != nil {
		t.Fatalf("AppendManagedAuthorizedKey() error = %v", err)
	}
	if added {
		t.Fatal("expected added=false for duplicate marker")
	}
	if strings.Count(updated, "infractl-ephemeral:task-123") != 1 {
		t.Fatalf("expected one marker occurrence, got %q", updated)
	}
}

func TestRemoveManagedAuthorizedKeyRemovesOnlyManagedLine(t *testing.T) {
	existing := strings.Join([]string{
		"ssh-ed25519 AAAA user1@host",
		"ssh-rsa BBBB user2@host",
		"ssh-ed25519 CCCC temp@runner infractl-ephemeral:task-123",
		"ssh-ed25519 DDDD temp2@runner infractl-ephemeral:task-456",
		"",
	}, "\n")

	updated, removed := RemoveManagedAuthorizedKey(existing, "task-123")
	if !removed {
		t.Fatal("expected removed=true")
	}
	if strings.Contains(updated, "infractl-ephemeral:task-123") {
		t.Fatalf("target managed key still present: %q", updated)
	}
	if !strings.Contains(updated, "ssh-ed25519 AAAA user1@host") {
		t.Fatalf("existing key removed unexpectedly: %q", updated)
	}
	if !strings.Contains(updated, "infractl-ephemeral:task-456") {
		t.Fatalf("other managed marker removed unexpectedly: %q", updated)
	}
}

func TestRemoveManagedAuthorizedKeyReturnsEmptyWhenOnlyManagedKeyExists(t *testing.T) {
	existing := "ssh-ed25519 CCCC temp@runner infractl-ephemeral:task-123\n"

	updated, removed := RemoveManagedAuthorizedKey(existing, "task-123")
	if !removed {
		t.Fatal("expected removed=true")
	}
	if updated != "" {
		t.Fatalf("expected empty authorized_keys after removal, got %q", updated)
	}
}
