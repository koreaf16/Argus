// Package session provides session ID and snapshot helpers.
//
// File role: Defines the persisted session snapshot schema (v1/v2) and a UUIDv4-style
// session ID generator for new Argus sessions.
//
// v1: Messages []llm.Message (기존)
// v2: Version=2, Graph []context.Node, Artifacts []context.ArtifactRef (신규)
package session

import (
	"crypto/rand"
	"fmt"
	"time"

	ctxpkg "github.com/koreaf16/argus/internal/context"
	"github.com/koreaf16/argus/internal/services/llm"
)

const SnapshotVersion = 2

// Snapshot is the persisted session payload.
// Version 1: Messages만 있음 (기존).
// Version 2: Version=2, Graph+Artifacts 추가됨.
type Snapshot struct {
	Version  int           `json:"version,omitempty"` // 0 또는 1 = v1, 2 = v2
	SavedAt  time.Time     `json:"saved_at"`
	Messages []llm.Message `json:"messages"` // v1 호환 유지

	// v2 전용 필드
	Graph     []*ctxpkg.Node        `json:"graph,omitempty"`
	Artifacts []*ctxpkg.ArtifactRef `json:"artifacts,omitempty"`

	// AppState 필드
	Mode     string                 `json:"mode,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// IsV2 는 이 snapshot이 v2 형식인지 반환한다.
func (s *Snapshot) IsV2() bool {
	return s.Version >= SnapshotVersion
}

// NewID returns a UUIDv4-compatible session ID.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf(
		"%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[0], b[1], b[2], b[3],
		b[4], b[5],
		b[6], b[7],
		b[8], b[9],
		b[10], b[11], b[12], b[13], b[14], b[15],
	), nil
}
