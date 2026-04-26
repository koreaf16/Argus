//go:build tools
// +build tools

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/yourorg/infractl/internal/store"
)

func main() {
	ctx := context.Background()
	dbPath := ".infractl/infractl.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}
	fmt.Printf("db=%s\n", dbPath)
	st, err := store.NewSQLiteStore(ctx, dbPath)
	if err != nil {
		panic(err)
	}
	defer st.Close()

	servers, err := st.List(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("== servers ==")
	for _, s := range servers {
		fmt.Printf("%s host=%s port=%d user=%s auth=%s os=%s workspace=%s\n", s.Name, s.Host, s.Port, s.User, s.AuthType, s.OS, s.WorkspaceDir)
	}

	connectors, err := st.ListConnectors(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println("== connectors ==")
	for _, c := range connectors {
		fmt.Printf("%s/%s/%s save=%s config=%s tools=%s\n", c.ServerName, c.ServiceType, c.ServiceName, c.SaveMode, c.Config, c.Tools)
	}

	fmt.Println("== discovered services ==")
	for _, s := range servers {
		services, err := st.ListDiscoveredServices(ctx, s.Name)
		if err != nil {
			panic(err)
		}
		for _, svc := range services {
			fmt.Printf("%s/%s/%s port=%d confidence=%.2f details=%v\n", svc.ServerName, svc.ServiceType, svc.ServiceName, svc.Port, svc.Confidence, svc.Details)
		}
	}
}
