// Package privilege
// File: vault_test.go
// Description: Vault 순수 로직 단위 테스트 (TTL, PurgeSession, 보안 로그 회귀 포함)
// Responsibility: Register/Lookup 라운드트립, TTL 만료, PurgeSession, 비밀번호 로그 미출력 검증

package privilege

import (
	"bytes"
	"log/slog"
	"testing"
	"time"
)

func newTestVault(fakeClock func() time.Time) *Vault {
	v := NewVault(NewCache())
	v.clock = fakeClock
	return v
}

// --- Register + Lookup round-trip ---

func TestVault_RegisterLookup_RoundTrip(t *testing.T) {
	now := time.Now()
	v := newTestVault(func() time.Time { return now })

	if err := v.Register("sess1", "dbhost", MethodSudo, "oracle", "s3cr3t", time.Hour); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	pw, ok := v.Lookup("dbhost", MethodSudo, "oracle")
	if !ok {
		t.Fatal("expected Lookup to return true")
	}
	if pw != "s3cr3t" {
		t.Fatalf("expected password %q, got %q", "s3cr3t", pw)
	}
}

// --- Register with empty sessionID must error ---

func TestVault_Register_EmptySessionID(t *testing.T) {
	v := newTestVault(time.Now)
	err := v.Register("", "dbhost", MethodSudo, "oracle", "pass", time.Hour)
	if err == nil {
		t.Fatal("expected error for empty session id")
	}
}

// --- TTL expiry ---

func TestVault_Lookup_TTLExpired(t *testing.T) {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	current := base
	v := newTestVault(func() time.Time { return current })

	const ttl = 5 * time.Minute
	if err := v.Register("sess1", "host1", MethodSU, "admin", "mypwd", ttl); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// advance clock past TTL
	current = base.Add(ttl + time.Second)

	_, ok := v.Lookup("host1", MethodSU, "admin")
	if ok {
		t.Fatal("expected Lookup to return false after TTL expired")
	}

	// entry must have been removed from the internal map
	v.mu.RLock()
	key := cacheKey("host1", MethodSU, "admin")
	_, stillPresent := v.entries[key]
	v.mu.RUnlock()
	if stillPresent {
		t.Fatal("expected expired entry to be removed from entries map")
	}
}

// --- PurgeSession ---

func TestVault_PurgeSession(t *testing.T) {
	now := time.Now()
	v := newTestVault(func() time.Time { return now })

	if err := v.Register("sess1", "host1", MethodSudo, "root", "rootpwd", time.Hour); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	v.PurgeSession("sess1")

	_, ok := v.Lookup("host1", MethodSudo, "root")
	if ok {
		t.Fatal("expected Lookup to return false after PurgeSession")
	}
}

// --- Security log regression: password must never appear in slog output ---

func TestVault_NoPasswordInLogs(t *testing.T) {
	const secretPassword = "SuperSecret123!"

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))

	now := time.Now()
	v := newTestVault(func() time.Time { return now })

	if err := v.Register("audit-sess", "securehost", MethodSudo, "oracle", secretPassword, time.Hour); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	_, _ = v.Lookup("securehost", MethodSudo, "oracle")
	v.PurgeSession("audit-sess")

	logOutput := buf.String()
	if contains := func(s, sub string) bool {
		return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
	}(logOutput, secretPassword); contains {
		t.Fatalf("password appeared in log output:\n%s", logOutput)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- List defensive copy ---

func TestVault_List_DefensiveCopy(t *testing.T) {
	now := time.Now()
	v := newTestVault(func() time.Time { return now })

	if err := v.Register("s1", "h1", MethodSudo, "admin", "pw1", time.Hour); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if err := v.Register("s1", "h2", MethodSU, "oracle", "pw2", time.Hour); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	list := v.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}

	// mutate the returned slice — internal state must not change
	list[0] = VaultEntry{Target: "MUTATED"}
	list = append(list, VaultEntry{Target: "EXTRA"})

	after := v.List()
	if len(after) != 2 {
		t.Fatalf("expected internal list still 2 after mutation, got %d", len(after))
	}
	for _, e := range after {
		if e.Target == "MUTATED" || e.Target == "EXTRA" {
			t.Fatal("external mutation affected internal vault state")
		}
	}
}

// --- Default TTL applied when ttl=0 ---

func TestVault_Register_DefaultTTL(t *testing.T) {
	now := time.Now()
	v := newTestVault(func() time.Time { return now })

	if err := v.Register("s1", "h1", MethodSudo, "root", "pw", 0); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	key := cacheKey("h1", MethodSudo, "root")
	v.mu.RLock()
	entry := v.entries[key]
	v.mu.RUnlock()

	if entry.TTL != defaultTTL {
		t.Fatalf("expected default TTL %v, got %v", defaultTTL, entry.TTL)
	}
}
