//go:build tools
// +build tools

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	replacements := map[string]string{
		"internal/workspace": "internal/serverenv",
		"workspace.":         "serverenv.",
		"WorkspaceDir":       "ServerDir",
		"workspaceDir":       "serverDir",
		"workspace_dir":      "server_dir",
		"workspace_add":      "server_add",
		"workspace_list":     "server_list",
		"workspace_remove":   "server_remove",
		"workspace_focus":    "server_focus",
		"Active workspace:":  "Active server:",
		"Workspace=":         "Server=",
		"Workspace '":        "Server '",
	}

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || name == "bin" || name == "scratch" {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".md") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		newContent := string(content)
		changed := false
		for old, new := range replacements {
			if strings.Contains(newContent, old) {
				newContent = strings.ReplaceAll(newContent, old, new)
				changed = true
			}
		}

		if changed {
			fmt.Printf("Updating %s\n", path)
			if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
