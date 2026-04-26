//go:build tools
// +build tools

// Package main
// File: scratch/decrypt_test.go
package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/yourorg/infractl/internal/config"
	"github.com/yourorg/infractl/internal/crypto"
	"github.com/yourorg/infractl/internal/store"
)

func main() {
	ctx := context.Background()
	configDir, _ := config.DefaultConfigDir()
	dbPath := filepath.Join(configDir, "infractl.db")
	sqliteStore, _ := store.NewSQLiteStore(ctx, dbPath)
	defer sqliteStore.Close()

	srv, err := sqliteStore.Get(ctx, "sandbox")
	if err != nil {
		log.Fatalf("failed to get server info: %v", err)
	}

	key, _ := crypto.DeriveKey()
	pw, err := crypto.Decrypt(key, srv.Credential)
	if err != nil {
		log.Fatalf("decrypt failed: %v", err)
	}

	fmt.Printf("Decrypted Password: [%s]\n", pw)
}
