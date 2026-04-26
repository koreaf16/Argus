//go:build tools
// +build tools

// Package main
// File: scratch/file_upload.go
// Description: Oracle 설치 파일을 sandbox 서버로 전송
// Responsibility: Use infractl internal components to transfer files

package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/yourorg/infractl/internal/config"
	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/executor"
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

	// build deps logic simplified
	execMgr := executor.NewManager(executor.NewLocalExecutor(0))
	connMgr := connector.NewManager(nil, sqliteStore)
	
	// register connector factories would be needed for SSH, but we can also use connectorMgr.LoadSaved
	// for simplicity, let's just use the SSH connector directly if possible or the manager
	if err := connMgr.LoadSaved(ctx); err != nil {
		log.Fatalf("load connectors: %v", err)
	}

	// Get Executor for sandbox
	exec, err := execMgr.Get(srv.Name)
	if err != nil {
		// Manager doesn't have it yet, we need to register it.
		// In infractl, connectorMgr.LoadSaved should have registered executors into the manager if they were connected.
		// Let's try to get it from connector manager if available.
		log.Fatalf("executor not found in manager: %v", err)
	}

	ft, ok := exec.(executor.FileTransferExecutor)
	if !ok {
		log.Fatalf("executor does not support file transfer")
	}

	files := []struct {
		src string
		dst string
	}{
		{"C:/Users/jhkwa/Downloads/oracle-database-preinstall-19c-1.0-1.el9.x86_64.rpm", "/tmp/preinstall.rpm"},
		{"C:/Users/jhkwa/Downloads/LINUX.X64_193000_db_home.zip", "/tmp/LINUX.X64_193000_db_home.zip"},
		{"C:/Users/jhkwa/Downloads/p37960098_190000_Linux-x86-64.zip", "/tmp/p37960098.zip"},
		{"C:/Users/jhkwa/Downloads/p6880880_190000_Linux-x86-64.zip", "/tmp/p6880880.zip"},
	}

	for _, f := range files {
		fmt.Printf("Uploading %s to %s...\n", f.src, f.dst)
		err := ft.Upload(ctx, f.src, f.dst, func(transferred, total int64) {
			if total > 0 {
				fmt.Printf("\rProgress: %d%% (%d/%d)", transferred*100/total, transferred, total)
			}
		})
		fmt.Println()
		if err != nil {
			fmt.Printf("Failed to upload %s: %v\n", f.src, err)
		}
	}
}
