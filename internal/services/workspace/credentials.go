package workspace

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/koreaf16/argus/internal/security"
)

type credEntry struct {
	Kind       string    `json:"kind"`
	BlobBase64 string    `json:"blob_base64"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type credCatalog struct {
	Version int                  `json:"version"`
	Entries map[string]credEntry `json:"entries,omitempty"`
}

// CredentialStore persists encrypted passwords to ssh_credentials.json.
// Encryption is delegated to a security.Encrypter (DPAPI on Windows).
type CredentialStore struct {
	mu      sync.RWMutex
	path    string
	enc     security.Encrypter
	entries map[string]credEntry
}

// NewCredentialStore creates a store backed by the given file and encrypter.
func NewCredentialStore(path string, enc security.Encrypter) *CredentialStore {
	return &CredentialStore{
		path:    path,
		enc:     enc,
		entries: make(map[string]credEntry),
	}
}

// Load reads the credentials file. If the file does not exist, the store starts empty.
func (c *CredentialStore) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			c.entries = make(map[string]credEntry)
			return nil
		}
		return fmt.Errorf("read %s: %w", c.path, err)
	}

	var cat credCatalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return fmt.Errorf("parse %s: %w", c.path, err)
	}
	if cat.Entries == nil {
		cat.Entries = make(map[string]credEntry)
	}
	c.entries = cat.Entries
	return nil
}

// SetPassword encrypts password and stores it under alias+kind.
// Call Save() afterwards to persist to disk.
func (c *CredentialStore) SetPassword(alias, kind, plaintext string) error {
	blob, err := c.enc.Encrypt([]byte(plaintext))
	if err != nil {
		return fmt.Errorf("encrypt credential for %s: %w", alias, err)
	}

	key := alias + "\x00" + kind
	c.mu.Lock()
	c.entries[key] = credEntry{
		Kind:       kind,
		BlobBase64: base64.StdEncoding.EncodeToString(blob),
		UpdatedAt:  time.Now().UTC(),
	}
	c.mu.Unlock()
	return nil
}

// GetPassword retrieves and decrypts the stored password for alias+kind.
// Returns ("", false, nil) when no entry exists.
func (c *CredentialStore) GetPassword(alias, kind string) (string, bool, error) {
	key := alias + "\x00" + kind

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return "", false, nil
	}

	blob, err := base64.StdEncoding.DecodeString(entry.BlobBase64)
	if err != nil {
		return "", true, fmt.Errorf("decode credential for %s: %w", alias, err)
	}

	plain, err := c.enc.Decrypt(blob)
	if err != nil {
		return "", true, fmt.Errorf("decrypt credential for %s: %w", alias, err)
	}
	return string(plain), true, nil
}

// Delete removes all stored entries for the given alias.
// Call Save() afterwards to persist to disk.
func (c *CredentialStore) Delete(alias string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := alias + "\x00"
	for key := range c.entries {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			delete(c.entries, key)
		}
	}
}

// Save atomically writes the credentials file (0o600).
func (c *CredentialStore) Save() error {
	c.mu.RLock()
	entries := make(map[string]credEntry, len(c.entries))
	for k, v := range c.entries {
		entries[k] = v
	}
	c.mu.RUnlock()

	cat := credCatalog{Version: 1, Entries: entries}
	payload, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.path); err != nil {
		// Fallback for Windows cross-device rename restrictions.
		if werr := os.WriteFile(c.path, payload, 0o600); werr != nil {
			return werr
		}
		_ = os.Remove(tmp)
	}
	return nil
}
