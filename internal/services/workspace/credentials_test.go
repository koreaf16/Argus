package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/koreaf16/argus/internal/security"
)

// xorEncrypter is a trivial mock Encrypter for tests (XOR with 0x42).
type xorEncrypter struct{}

func (xorEncrypter) Encrypt(b []byte) ([]byte, error) {
	out := make([]byte, len(b))
	for i, v := range b {
		out[i] = v ^ 0x42
	}
	return out, nil
}
func (xorEncrypter) Decrypt(b []byte) ([]byte, error) { return xorEncrypter{}.Encrypt(b) }

type failEncrypter struct{}

func (failEncrypter) Encrypt(_ []byte) ([]byte, error) {
	return nil, errors.New("encrypt failure")
}
func (failEncrypter) Decrypt(_ []byte) ([]byte, error) {
	return nil, errors.New("decrypt failure")
}

func newTestStore(t *testing.T, enc security.Encrypter) (*CredentialStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ssh_credentials.json")
	store := NewCredentialStore(path, enc)
	if err := store.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store, path
}

func TestCredentialStoreRoundtrip(t *testing.T) {
	store, path := newTestStore(t, xorEncrypter{})

	if err := store.SetPassword("myserver", "ssh", "s3cr3t"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}

	store2 := NewCredentialStore(path, xorEncrypter{})
	if err := store2.Load(); err != nil {
		t.Fatal(err)
	}

	pw, ok, err := store2.GetPassword("myserver", "ssh")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected entry to exist after reload")
	}
	if pw != "s3cr3t" {
		t.Fatalf("got %q, want %q", pw, "s3cr3t")
	}
}

func TestCredentialStoreMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")
	store := NewCredentialStore(path, xorEncrypter{})
	if err := store.Load(); err != nil {
		t.Fatalf("Load on missing file should not error: %v", err)
	}
	_, ok, err := store.GetPassword("any", "ssh")
	if err != nil || ok {
		t.Fatalf("expected empty store: ok=%v err=%v", ok, err)
	}
}

func TestCredentialStoreDelete(t *testing.T) {
	store, _ := newTestStore(t, xorEncrypter{})
	_ = store.SetPassword("srv", "ssh", "pw1")
	store.Delete("srv")
	_, ok, _ := store.GetPassword("srv", "ssh")
	if ok {
		t.Fatal("entry should be gone after Delete")
	}
}

func TestCredentialStoreEncryptFailure(t *testing.T) {
	store, _ := newTestStore(t, failEncrypter{})
	if err := store.SetPassword("srv", "ssh", "pw"); err == nil {
		t.Fatal("expected encrypt error")
	}
}

func TestCredentialStoreDecryptFailure(t *testing.T) {
	// Write entry with one encrypter, read with another (simulates key mismatch).
	store, path := newTestStore(t, xorEncrypter{})
	_ = store.SetPassword("srv", "ssh", "pw")
	_ = store.Save()

	store2 := NewCredentialStore(path, failEncrypter{})
	_ = store2.Load()

	_, ok, err := store2.GetPassword("srv", "ssh")
	if !ok || err == nil {
		t.Fatalf("expected ok=true err!=nil, got ok=%v err=%v", ok, err)
	}
}

func TestCredentialStoreSaveAtomic(t *testing.T) {
	store, path := newTestStore(t, xorEncrypter{})
	_ = store.SetPassword("srv", "ssh", "pw")
	if err := store.Save(); err != nil {
		t.Fatal(err)
	}
	// tmp file should be cleaned up
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("tmp file should not exist after successful save")
	}
}
