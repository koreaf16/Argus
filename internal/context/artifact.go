// 파일 역할: ArtifactStore — 대형 tool output의 raw 내용을 파일로 저장/로드.
//
//	~/<baseDir>/session-artifacts/<sessionID>/<seq>-<tool>-<callID_prefix>.txt
package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

// ArtifactRef 는 저장된 artifact에 대한 참조다.
type ArtifactRef struct {
	ID       string `json:"id"`        // "<seq>-<toolName>-<callID_prefix>"
	Path     string `json:"path"`      // 절대 파일 경로
	Chars    int    `json:"chars"`     // 원본 문자 수
	ToolName string `json:"tool_name"` // 어느 tool에서 생성됐는지
	CallID   string `json:"call_id"`   // tool call ID
}

// ArtifactManifest 는 세션 내 모든 artifact 목록이다.
type ArtifactManifest struct {
	mu   sync.RWMutex
	refs map[string]*ArtifactRef // key: ArtifactRef.ID
}

// NewArtifactManifest 는 빈 manifest를 반환한다.
func NewArtifactManifest() *ArtifactManifest {
	return &ArtifactManifest{refs: make(map[string]*ArtifactRef)}
}

// Add 는 manifest에 새 artifact ref를 추가한다.
func (m *ArtifactManifest) Add(ref *ArtifactRef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.refs[ref.ID] = ref
}

// Get 은 ID로 artifact ref를 반환한다.
func (m *ArtifactManifest) Get(id string) (*ArtifactRef, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.refs[id]
	return r, ok
}

// All 은 현재 manifest의 모든 ref 슬라이스를 반환한다.
func (m *ArtifactManifest) All() []*ArtifactRef {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*ArtifactRef, 0, len(m.refs))
	for _, r := range m.refs {
		out = append(out, r)
	}
	return out
}

// ArtifactStore 는 artifact 파일의 read/write를 담당한다.
type ArtifactStore struct {
	baseDir   string // 예: ~/.argus
	sessionID string
}

// NewArtifactStore 는 새 ArtifactStore를 반환한다.
func NewArtifactStore(baseDir, sessionID string) *ArtifactStore {
	return &ArtifactStore{baseDir: baseDir, sessionID: sessionID}
}

// artifactsDir 는 세션별 artifact 저장 디렉터리를 반환한다.
func (s *ArtifactStore) artifactsDir() string {
	return filepath.Join(s.baseDir, "session-artifacts", s.sessionID)
}

// Bootstrap 은 artifact 디렉터리를 생성한다.
func (s *ArtifactStore) Bootstrap() error {
	return os.MkdirAll(s.artifactsDir(), 0o755)
}

// Save 는 raw 내용을 파일로 저장하고 ArtifactRef를 반환한다.
func (s *ArtifactStore) Save(seq int, toolName, callID, content string) (*ArtifactRef, error) {
	if err := s.Bootstrap(); err != nil {
		return nil, fmt.Errorf("artifact bootstrap: %w", err)
	}

	safeTool := safeFilenameComponent(toolName, 24)
	safeCall := safeFilenameComponent(callID, 8)
	id := fmt.Sprintf("%d-%s-%s", seq, safeTool, safeCall)
	filename := id + ".txt"

	base := filepath.Clean(s.artifactsDir())
	path := filepath.Join(base, filename)
	// 경로 탈출 방지: filepath.Join이 ..를 정규화한 결과가 base 안에 있는지 검증
	if !strings.HasPrefix(path+string(filepath.Separator), base+string(filepath.Separator)) {
		return nil, fmt.Errorf("artifact path escaped base dir: %s", path)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return nil, fmt.Errorf("artifact write %s: %w", path, err)
	}

	return &ArtifactRef{
		ID:       id,
		Path:     path,
		Chars:    len(content),
		ToolName: toolName,
		CallID:   callID,
	}, nil
}

// safeFilenameComponent 는 문자열에서 영숫자·_·- 만 남기고 maxLen으로 자른다.
// 경로 구분자, .., 제어 문자 등을 모두 제거해 파일명 traversal을 방지한다.
func safeFilenameComponent(s string, maxLen int) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
		if b.Len() >= maxLen {
			break
		}
	}
	result := b.String()
	if result == "" {
		return "unknown"
	}
	return result
}

// Load 는 artifact 파일의 내용을 반환한다.
func (s *ArtifactStore) Load(id string) (string, error) {
	refs, _ := s.listFiles()
	for _, f := range refs {
		if strings.HasPrefix(filepath.Base(f), id) {
			data, err := os.ReadFile(f)
			if err != nil {
				return "", err
			}
			return string(data), nil
		}
	}
	return "", fmt.Errorf("artifact %q not found", id)
}

// listFiles 는 artifact 디렉터리의 파일 경로 목록을 반환한다.
func (s *ArtifactStore) listFiles() ([]string, error) {
	entries, err := os.ReadDir(s.artifactsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, filepath.Join(s.artifactsDir(), e.Name()))
		}
	}
	return out, nil
}
