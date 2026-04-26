//go:build tools
// +build tools

package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"os"

	"github.com/yourorg/infractl/internal/store"
)

func main() {
	userHome, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	dbPath := filepath.Join(userHome, ".infractl", "infractl.db")

	ctx := context.Background()
	st, err := store.NewSQLiteStore(ctx, dbPath)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	name := "oracle"
	err = st.Remove(ctx, name)
	if err != nil {
		log.Fatalf("failed to remove workspace '%s': %v", name, err)
	}

	fmt.Printf("Workspace '%s' removed successfully from %s\n", name, dbPath)
}
