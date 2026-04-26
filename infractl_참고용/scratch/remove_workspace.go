//go:build tools
// +build tools

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
	configDir, err := config.DefaultConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	dbPath := filepath.Join(configDir, "infractl.db")

	s, err := store.NewSQLiteStore(ctx, dbPath)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	name := "oracle"
	err = s.Remove(ctx, name)
	if err != nil {
		log.Fatalf("failed to remove workspace '%s': %v", name, err)
	}

	fmt.Printf("Workspace '%s' has been successfully removed from infractl.db\n", name)
}
