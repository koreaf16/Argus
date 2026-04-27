package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type PasswordRequestFunc func(alias, kind, prompt string) (string, error)

type Manager struct {
	reg      *Registry
	creds    *CredentialStore
	sessions map[string]*sshSession
	pwCache  map[string]map[string]string // alias -> kind -> password
	mu       sync.RWMutex
	active   string

	promptFn PasswordRequestFunc

	inspectCache map[string]InspectSnapshot
}

func NewManager(reg *Registry, promptFn PasswordRequestFunc) *Manager {
	return &Manager{
		reg:          reg,
		sessions:     make(map[string]*sshSession),
		pwCache:      make(map[string]map[string]string),
		inspectCache: make(map[string]InspectSnapshot),
		promptFn:     promptFn,
		active:       LocalAlias,
	}
}

func (m *Manager) SetCredentialStore(creds *CredentialStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creds = creds
}

func (m *Manager) SetPromptFunc(fn PasswordRequestFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.promptFn = fn
}

func (m *Manager) Registry() *Registry {
	return m.reg
}

func (m *Manager) ActiveAlias() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.active == "" {
		return LocalAlias
	}
	return m.active
}

func (m *Manager) ResolveAlias(alias string) string {
	if alias == "" || alias == "local" {
		return LocalAlias
	}
	return alias
}

func (m *Manager) GetServer(alias string) (ServerEntry, error) {
	alias = m.ResolveAlias(alias)
	entry, ok := m.reg.Get(alias)
	if !ok {
		return ServerEntry{}, fmt.Errorf("unknown server alias: %s", alias)
	}
	return entry, nil
}

func (m *Manager) UpsertServer(entry ServerEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reg.Add(entry) // Registry에 동적으로 추가
}

func (m *Manager) IsRemoteAlias(alias string) bool {
	alias = m.ResolveAlias(alias)
	if alias == LocalAlias {
		return false
	}
	entry, ok := m.reg.Get(alias)
	return ok && entry.Kind == ServerKindSSH
}

func (m *Manager) SetActive(alias string) error {
	alias = m.ResolveAlias(alias)
	if alias != LocalAlias {
		if _, ok := m.reg.Get(alias); !ok {
			return fmt.Errorf("unknown server alias: %s", alias)
		}
	}
	m.mu.Lock()
	m.active = alias
	m.mu.Unlock()
	return nil
}

func (m *Manager) SetInspectSnapshot(alias string, snapshot InspectSnapshot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inspectCache[alias] = snapshot
}

func (m *Manager) GetInspectSnapshot(alias string) (InspectSnapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res, ok := m.inspectCache[alias]
	return res, ok
}

func (m *Manager) Connect(alias string) error {
	alias = m.ResolveAlias(alias)
	if alias == LocalAlias {
		return nil
	}

	if existing := m.getSession(alias); existing != nil {
		if _, _, err := existing.client.SendRequest("keepalive@openssh.com", true, nil); err == nil {
			return nil
		}
		_ = m.Disconnect(alias)
	}

	entry, err := m.GetServer(alias)
	if err != nil {
		return err
	}

	session, err := newSSHSession(entry, m.requestPassword)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.sessions[alias] = session
	m.mu.Unlock()

	return nil
}

func (m *Manager) Disconnect(alias string) error {
	alias = m.ResolveAlias(alias)
	if alias == LocalAlias {
		return nil
	}

	var session *sshSession
	m.mu.Lock()
	session = m.sessions[alias]
	delete(m.sessions, alias)
	delete(m.pwCache, alias)
	delete(m.inspectCache, alias)
	m.mu.Unlock()

	if session != nil {
		return session.Close()
	}
	return nil
}

func (m *Manager) DisconnectAll() error {
	m.mu.Lock()
	aliases := make([]string, 0, len(m.sessions))
	for a := range m.sessions {
		aliases = append(aliases, a)
	}
	m.mu.Unlock()

	for _, a := range aliases {
		_ = m.Disconnect(a)
	}
	return nil
}

func (m *Manager) SetPassword(alias, kind, password string) {
	// [보안 및 무결성] UI 아티팩트가 비밀번호로 저장되는 것을 원천 차단
	if !isValidPassword(password) {
		return
	}

	m.cachePassword(alias, kind, password)

	if sessionID := os.Getenv("ARGUS_SESSION_ID"); sessionID != "" {
		m.saveTempCredential(sessionID, alias, kind, password)
	}

	m.mu.RLock()
	creds := m.creds
	m.mu.RUnlock()
	if creds != nil {
		_ = creds.SetPassword(alias, kind, password)
	}
}

func isValidPassword(pw string) bool {
	pw = strings.TrimSpace(pw)
	if pw == "" || len(pw) > 256 {
		return false
	}
	// 터미널 UI 특수문자 포함 여부 체크
	badChars := []string{"╭", "─", "╮", "│", "╰", "╯", "⊷", "✦"}
	for _, char := range badChars {
		if strings.Contains(pw, char) {
			return false
		}
	}
	return true
}

func (m *Manager) GetPassword(alias, kind string) string {
	return m.cachedPassword(alias, kind)
}

func (m *Manager) requestPassword(alias, kind, prompt string) (string, error) {
	if sessionID := os.Getenv("ARGUS_SESSION_ID"); sessionID != "" {
		m.loadTempCredentials(sessionID)
	}

	if cached := m.cachedPassword(alias, kind); cached != "" {
		return cached, nil
	}

	if kind == "su" || kind == "sudo" || kind == "ssh" {
		kinds := []string{"ssh", "sudo", "su"}
		for _, k := range kinds {
			if cached := m.cachedPassword(alias, k); cached != "" {
				return cached, nil
			}
		}
	}

	m.mu.RLock()
	creds := m.creds
	m.mu.RUnlock()
	if creds != nil {
		pw, ok, err := creds.GetPassword(alias, kind)
		if err == nil && ok && isValidPassword(pw) {
			m.cachePassword(alias, kind, pw)
			return pw, nil
		}
	}

	m.mu.RLock()
	fn := m.promptFn
	m.mu.RUnlock()
	if fn == nil {
		return "", fmt.Errorf("password required for %s but no interaction allowed in this mode", alias)
	}

	pw, err := fn(alias, kind, prompt)
	if err == nil && isValidPassword(pw) {
		m.SetPassword(alias, kind, pw)
	}
	return pw, err
}

func (m *Manager) cachePassword(alias, kind, password string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pwCache[alias] == nil {
		m.pwCache[alias] = make(map[string]string)
	}
	m.pwCache[alias][kind] = password
}

func (m *Manager) cachedPassword(alias, kind string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.pwCache[alias] == nil {
		return ""
	}
	return m.pwCache[alias][kind]
}

func (m *Manager) saveTempCredential(sessionID, alias, kind, password string) {
	artifactDir := filepath.Join(".Argus", "session-artifacts", sessionID)
	_ = os.MkdirAll(artifactDir, 0o700)
	tempFile := filepath.Join(artifactDir, "temp_credentials.json")

	m.mu.Lock()
	defer m.mu.Unlock()

	data := make(map[string]map[string]string)
	if bytes, err := os.ReadFile(tempFile); err == nil {
		_ = json.Unmarshal(bytes, &data)
	}

	if data[alias] == nil {
		data[alias] = make(map[string]string)
	}
	data[alias][kind] = password

	if bytes, err := json.MarshalIndent(data, "", "  "); err == nil {
		_ = os.WriteFile(tempFile, bytes, 0o600)
	}
}

func (m *Manager) loadTempCredentials(sessionID string) {
	tempFile := filepath.Join(".Argus", "session-artifacts", sessionID, "temp_credentials.json")
	bytes, err := os.ReadFile(tempFile)
	if err != nil {
		return
	}

	data := make(map[string]map[string]string)
	if err := json.Unmarshal(bytes, &data); err != nil {
		return
	}

	for alias, creds := range data {
		for kind, pw := range creds {
			m.cachePassword(alias, kind, pw)
		}
	}
}

func (m *Manager) Exec(ctx context.Context, alias, command string, subQuery ExecuteSubQueryFunc) (ExecResult, error) {
	return m.ExecWithOptions(ctx, alias, command, ExecOptions{}, subQuery)
}

func (m *Manager) StartExecWithOptions(ctx context.Context, alias, command string, opts ExecOptions, subQuery ExecuteSubQueryFunc) (*ExecHandle, error) {
	alias = m.ResolveAlias(alias)
	if opts.Password != "" {
		m.SetPassword(alias, "ssh", opts.Password)
	}
	if opts.RootPassword != "" {
		m.SetPassword(alias, "sudo", opts.RootPassword)
	}

	if !m.IsRemoteAlias(alias) {
		return m.startLocalExecStreaming(ctx, command, opts)
	}

	session, err := m.ensureSession(alias)
	if err != nil {
		return nil, err
	}
	return session.StartExec(ctx, command, opts, subQuery)
}

func (m *Manager) ExecWithOptions(ctx context.Context, alias, command string, opts ExecOptions, subQuery ExecuteSubQueryFunc) (ExecResult, error) {
	alias = m.ResolveAlias(alias)
	if opts.Password != "" {
		m.SetPassword(alias, "ssh", opts.Password)
	}
	if opts.RootPassword != "" {
		m.SetPassword(alias, "sudo", opts.RootPassword)
	}

	if !m.IsRemoteAlias(alias) {
		return m.execLocal(ctx, command, opts)
	}

	session, err := m.ensureSession(alias)
	if err != nil {
		return ExecResult{}, err
	}
	return session.Exec(ctx, command, opts, subQuery)
}

func (m *Manager) CurrentUser(alias string) string {
	if !m.IsRemoteAlias(alias) {
		return os.Getenv("USERNAME")
	}
	session := m.getSession(alias)
	if session == nil {
		return ""
	}
	return session.entry.User
}

func (m *Manager) ListDir(ctx context.Context, alias, root string, recursive bool, depth int) ([]FileEntry, error) {
	alias = m.ResolveAlias(alias)
	if alias == LocalAlias {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		var files []FileEntry
		for _, e := range entries {
			info, _ := e.Info()
			files = append(files, FileEntry{
				Name:    e.Name(),
				Path:    filepath.Join(root, e.Name()),
				IsDir:   e.IsDir(),
				Size:    info.Size(),
				Mode:    info.Mode().String(),
				ModTime: info.ModTime(),
			})
		}
		return files, nil
	}
	session, err := m.ensureSession(alias)
	if err != nil {
		return nil, err
	}
	return session.listDirSFTP(root, recursive, depth)
}

func (m *Manager) ReadFile(ctx context.Context, alias, path string) ([]byte, error) {
	alias = m.ResolveAlias(alias)
	if alias == LocalAlias {
		return os.ReadFile(path)
	}
	session, err := m.ensureSession(alias)
	if err != nil {
		return nil, err
	}
	return session.readFileSFTP(path)
}

func (m *Manager) WriteFile(ctx context.Context, alias, path string, content []byte, overwrite bool) error {
	if !m.IsRemoteAlias(alias) {
		return os.WriteFile(path, content, 0o644)
	}
	session, err := m.ensureSession(alias)
	if err != nil {
		return err
	}
	return session.writeFileSFTP(path, content, overwrite)
}

func (m *Manager) CopyFile(ctx context.Context, srcAlias, srcPath, dstAlias, dstPath string, overwrite bool) error {
	src, err := m.openReader(srcAlias, srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := m.openWriter(dstAlias, dstPath, overwrite)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func (m *Manager) openReader(alias, path string) (io.ReadCloser, error) {
	alias = m.ResolveAlias(alias)
	if alias == LocalAlias {
		return os.Open(path)
	}
	session, err := m.ensureSession(alias)
	if err != nil {
		return nil, err
	}
	return session.openFileForReadSFTP(path)
}

func (m *Manager) openWriter(alias, path string, overwrite bool) (io.WriteCloser, error) {
	alias = m.ResolveAlias(alias)
	if alias == LocalAlias {
		return os.Create(path)
	}
	session, err := m.ensureSession(alias)
	if err != nil {
		return nil, err
	}
	return session.openFileForWriteSFTP(path, overwrite)
}

func (m *Manager) OpenTunnel(ctx context.Context, alias, localAddr, remoteAddr string) (TunnelInfo, error) {
	session, err := m.ensureSession(alias)
	if err != nil {
		return TunnelInfo{}, err
	}
	return session.openTunnel(localAddr, remoteAddr)
}

func (m *Manager) CloseTunnel(alias, tunnelID string) error {
	session := m.getSession(alias)
	if session == nil {
		return fmt.Errorf("session not found")
	}
	return session.closeTunnel(tunnelID)
}

func (m *Manager) ListTunnels(alias string) ([]TunnelInfo, error) {
	session := m.getSession(alias)
	if session == nil {
		return nil, fmt.Errorf("session not found")
	}
	return session.listTunnels(), nil
}

func (m *Manager) MetricsSnapshot(ctx context.Context, alias string) (MetricsSnapshot, error) {
	return MetricsSnapshot{Alias: alias, CollectedAt: time.Now()}, nil
}

func (m *Manager) Status() []StatusEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	entries := m.reg.List()
	var res []StatusEntry
	
	// Add local workspace first
	res = append(res, StatusEntry{
		Alias:     LocalAlias,
		Kind:      ServerKindLocal,
		Connected: true,
		User:      os.Getenv("USERNAME"),
	})

	for _, e := range entries {
		session, connected := m.sessions[e.Alias]
		user := e.User
		if connected && session.entry.User != "" {
			user = session.entry.User
		}
		
		res = append(res, StatusEntry{
			Alias:     e.Alias,
			Kind:      e.Kind,
			Connected: connected,
			User:      user,
		})
	}
	return res
}

func (m *Manager) ensureSession(alias string) (*sshSession, error) {
	if err := m.Connect(alias); err != nil {
		return nil, err
	}
	session := m.getSession(alias)
	if session == nil {
		return nil, fmt.Errorf("ssh session unavailable for %s", alias)
	}
	return session, nil
}

func (m *Manager) getSession(alias string) *sshSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[alias]
}
