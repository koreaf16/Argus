package connector

import (
	"context"
	"testing"

	"github.com/yourorg/infractl/internal/store"
)

type fakeConnectorStore struct{}

func (fakeConnectorStore) SaveConnector(context.Context, store.ConnectorEntry) error {
	return nil
}
func (fakeConnectorStore) ListConnectors(context.Context) ([]store.ConnectorEntry, error) {
	return nil, nil
}
func (fakeConnectorStore) ListPermanentConnectors(context.Context) ([]store.ConnectorEntry, error) {
	return nil, nil
}
func (fakeConnectorStore) GetConnector(context.Context, string, string, string) (store.ConnectorEntry, error) {
	return store.ConnectorEntry{}, nil
}
func (fakeConnectorStore) RemoveConnector(context.Context, string, string, string) error {
	return nil
}
func (fakeConnectorStore) EncryptConnectorCreds(plaintext string) (string, error) {
	return plaintext, nil
}
func (fakeConnectorStore) DecryptConnectorCreds(ciphertext string) (string, error) {
	return ciphertext, nil
}

func TestDecryptCredsInfersLegacyOracleOSAuth(t *testing.T) {
	mgr := NewManager(nil, fakeConnectorStore{})

	creds, err := mgr.decryptCreds(`{"username":"/","password":"","role":"sysdba"}`)
	if err != nil {
		t.Fatalf("decryptCreds failed: %v", err)
	}
	if !creds.OSAuth {
		t.Fatalf("expected OSAuth to be inferred for legacy / as sysdba credentials: %+v", creds)
	}
	if creds.Username != "/" || creds.Role != "sysdba" {
		t.Fatalf("unexpected credentials: %+v", creds)
	}
}
