// Package store
// File: history_paste.go
// Description: prompt history pasted text serialization and external cache helpers
// Responsibility: normalize prompt-history pasted content for persistence and restore
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/yourorg/infractl/internal/config"
)

const (
	maxInlinePastedContentLength = 1024
	pasteCacheDirName            = "paste-cache"
)

// PastedContent is an inline-ref text payload associated with a prompt draft.
type PastedContent struct {
	ID      int    `json:"id"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type storedPastedContent struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Content     string `json:"content,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
}

func clonePastedContents(contents map[int]PastedContent) map[int]PastedContent {
	if len(contents) == 0 {
		return nil
	}

	cloned := make(map[int]PastedContent, len(contents))
	for id, content := range contents {
		cloned[id] = content
	}
	return cloned
}

func maskPastedContents(contents map[int]PastedContent) map[int]PastedContent {
	if len(contents) == 0 {
		return nil
	}

	masked := make(map[int]PastedContent, len(contents))
	for id, content := range contents {
		content.Content = MaskSensitive(content.Content)
		masked[id] = content
	}
	return masked
}

func marshalStoredPastedContents(contents map[int]PastedContent) (string, error) {
	if len(contents) == 0 {
		return "", nil
	}

	ids := make([]int, 0, len(contents))
	for id := range contents {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	stored := make([]storedPastedContent, 0, len(contents))
	for _, id := range ids {
		content := contents[id]
		entry := storedPastedContent{
			ID:   content.ID,
			Type: content.Type,
		}

		if len(content.Content) > maxInlinePastedContentLength {
			hash, err := storePastedText(content.Content)
			if err != nil {
				return "", fmt.Errorf("store pasted text %d: %w", id, err)
			}
			entry.ContentHash = hash
		} else {
			entry.Content = content.Content
		}

		stored = append(stored, entry)
	}

	data, err := json.Marshal(stored)
	if err != nil {
		return "", fmt.Errorf("marshal pasted contents: %w", err)
	}
	return string(data), nil
}

func unmarshalStoredPastedContents(data string) (map[int]PastedContent, error) {
	if data == "" {
		return nil, nil
	}

	var stored []storedPastedContent
	if err := json.Unmarshal([]byte(data), &stored); err != nil {
		return nil, fmt.Errorf("unmarshal pasted contents: %w", err)
	}

	if len(stored) == 0 {
		return nil, nil
	}

	contents := make(map[int]PastedContent, len(stored))
	for _, entry := range stored {
		content := PastedContent{
			ID:   entry.ID,
			Type: entry.Type,
		}
		if entry.ContentHash != "" {
			text, err := loadPastedText(entry.ContentHash)
			if err != nil {
				return nil, fmt.Errorf("load pasted text %d: %w", entry.ID, err)
			}
			content.Content = text
		} else {
			content.Content = entry.Content
		}
		contents[entry.ID] = content
	}
	return contents, nil
}

func storePastedText(content string) (string, error) {
	hash := hashPastedText(content)
	path, err := pasteCachePath(hash)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("write pasted text %s: %w", path, err)
	}
	return hash, nil
}

func loadPastedText(hash string) (string, error) {
	path, err := pasteCachePath(hash)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read pasted text %s: %w", path, err)
	}
	return string(data), nil
}

func pasteCachePath(hash string) (string, error) {
	configDir, err := config.DefaultConfigDir()
	if err != nil {
		return "", fmt.Errorf("get config dir: %w", err)
	}

	dir := filepath.Join(configDir, pasteCacheDirName)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create paste cache dir: %w", err)
	}
	return filepath.Join(dir, hash+".txt"), nil
}

func hashPastedText(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
