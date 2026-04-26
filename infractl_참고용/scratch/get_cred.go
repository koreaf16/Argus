//go:build tools
// +build tools

// Package main
// File: scratch/get_cred.go
package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/yourorg/infractl/internal/config"
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

	fmt.Printf("Server: %s\nHost: %s\nUser: %s\nPassword: [%s]\n", srv.Name, srv.Host, srv.User, srv.Password)
}
