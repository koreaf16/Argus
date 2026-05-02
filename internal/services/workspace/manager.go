package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/koreaf16/argus/internal/services/workspace/channel"
)

type PasswordRequestFunc func(alias, kind, prompt string) (string, error)

type Manager struct {
	reg           *Registry
	creds         *CredentialStore
	sessions      map[string]*sshSession
	accountShells map[string]*accountShellSession
	pwCache       map[string]map[string]string // alias -> credential cache key -> password
	mu            sync.RWMutex
	active        string

	promptFn PasswordRequestFunc

	inspectCache map[string]InspectSnapshot

	// channel-based multi-channel SSH backbone (MobaXterm-style). Privilege x
	// purpose channels multiplex over a single master *ssh.Client per host.
	// Built lazily on first use so existing code paths keep functioning while
	// the redesign migrates incrementally.
	chanOnce sync.Once
	chanPool *channel.ConnectionPool
	chanMgr  channel.ChannelManager

	// activePriv tracks the privilege key currently held by the most recent
	// exec channel per alias, so RouteFor can place the next command on the
	// matching privilege lane without forcing a default-channel acquire.
	activePrivMu sync.RWMutex
	activePriv   map[string]channel.PrivilegeKey
}

// ExecutionTarget is the runtime resolution of a user-facing workspace alias.
// SSH entries resolve to themselves. Account entries resolve to their parent
// SSH host plus the target account reached through su/sudo on that host.
type ExecutionTarget struct {
	Alias        string
	HostAlias    string
	Entry        ServerEntry
	HostEntry    ServerEntry
	IsAccount    bool
	TargetUser   string
	SwitchMethod string
	DefaultCWD   string
}

func NewManager(reg *Registry, promptFn PasswordRequestFunc) *Manager {
	active := LocalAlias
	if reg != nil {
		active = reg.Active()
		if strings.TrimSpace(active) == "" {
			active = LocalAlias
		}
	}
	return &Manager{
		reg:           reg,
		sessions:      make(map[string]*sshSession),
		accountShells: make(map[string]*accountShellSession),
		pwCache:       make(map[string]map[string]string),
		inspectCache:  make(map[string]InspectSnapshot),
		promptFn:      promptFn,
		active:        active,
		activePriv:    make(map[string]channel.PrivilegeKey),
	}
}

// channelPool returns the lazily-built ConnectionPool, wiring the password
// prompt and credential clearer the first time a channel-aware caller asks
// for it. The legacy ssh_session path keeps working — channels and sessions
// share the same registry and password cache, so users see no double
// prompts during the migration window.
func (m *Manager) channelPool() *channel.ConnectionPool {
	m.chanOnce.Do(func() {
		m.chanPool = channel.NewConnectionPool(m, m.requestPassword, channelClearer{m: m})
		m.chanMgr = channel.NewChannelManager(m.chanPool)
	})
	return m.chanPool
}

// ChannelManager exposes the multi-channel backbone for tools that want to
// inspect snapshots or address a specific privilege lane. The returned
// interface is safe to use after Manager construction and is built lazily.
func (m *Manager) ChannelManager() channel.ChannelManager {
	m.channelPool()
	return m.chanMgr
}

// channelClearer adapts Manager's password cache to channel.CredentialClearer
// so AuthFailedError on the channel pool resets ssh credentials before the
// retry prompt fires.
type channelClearer struct{ m *Manager }

func (c channelClearer) ClearAlias(alias string) { c.m.clearPasswordCache(alias) }

// ActivePrivilege returns the privilege key the most recent exec channel
// for alias resolved to. Default for any alias never seen before. Exposed
// for the bash tool's channel_decision telemetry.
func (m *Manager) ActivePrivilege(alias string) channel.PrivilegeKey {
	m.activePrivMu.RLock()
	defer m.activePrivMu.RUnlock()
	if key, ok := m.activePriv[alias]; ok {
		return key
	}
	return channel.PrivilegeDefault
}

func (m *Manager) setActivePrivilege(alias string, key channel.PrivilegeKey) {
	m.activePrivMu.Lock()
	if m.activePriv == nil {
		m.activePriv = make(map[string]channel.PrivilegeKey)
	}
	m.activePriv[alias] = key
	m.activePrivMu.Unlock()
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

// ResolveExecutionTarget maps a user-facing alias to the actual SSH host used
// for transport and, when applicable, the account target entered on that host.
func (m *Manager) ResolveExecutionTarget(alias string) (ExecutionTarget, error) {
	alias = m.ResolveAlias(alias)
	entry, err := m.GetServer(alias)
	if err != nil {
		return ExecutionTarget{}, err
	}
	if alias == LocalAlias || entry.Kind == ServerKindLocal {
		return ExecutionTarget{
			Alias:      LocalAlias,
			HostAlias:  LocalAlias,
			Entry:      ServerEntry{Alias: LocalAlias, Kind: ServerKindLocal},
			HostEntry:  ServerEntry{Alias: LocalAlias, Kind: ServerKindLocal},
			DefaultCWD: entry.DefaultCWD,
		}, nil
	}
	switch entry.Kind {
	case ServerKindSSH:
		return ExecutionTarget{
			Alias:      entry.Alias,
			HostAlias:  entry.Alias,
			Entry:      entry,
			HostEntry:  entry,
			TargetUser: entry.User,
			DefaultCWD: entry.DefaultCWD,
		}, nil
	case ServerKindAccount:
		parentAlias := m.ResolveAlias(entry.ParentAlias)
		parent, ok := m.reg.Get(parentAlias)
		if !ok {
			return ExecutionTarget{}, fmt.Errorf("account target %s references unknown parent host: %s", entry.Alias, parentAlias)
		}
		if parent.Kind != ServerKindSSH {
			return ExecutionTarget{}, fmt.Errorf("account target %s parent %s is not an SSH host", entry.Alias, parentAlias)
		}
		defaultCWD := strings.TrimSpace(entry.DefaultCWD)
		if defaultCWD == "" {
			defaultCWD = parent.DefaultCWD
		}
		return ExecutionTarget{
			Alias:        entry.Alias,
			HostAlias:    parent.Alias,
			Entry:        entry,
			HostEntry:    parent,
			IsAccount:    true,
			TargetUser:   strings.TrimSpace(entry.User),
			SwitchMethod: normalizeSwitchMethod(entry.SwitchMethod),
			DefaultCWD:   defaultCWD,
		}, nil
	default:
		return ExecutionTarget{}, fmt.Errorf("alias %s is not executable workspace target", alias)
	}
}

// ResolveHostAlias returns the physical host alias used for SSH transport.
func (m *Manager) ResolveHostAlias(alias string) (string, error) {
	target, err := m.ResolveExecutionTarget(alias)
	if err != nil {
		return "", err
	}
	return target.HostAlias, nil
}

// ResolveEntry implements channel.EntryResolver. It maps a workspace alias to
// the lite EntrySpec the channel package consumes, rejecting non-SSH aliases
// so the channel pool never tries to dial a local workspace.
func (m *Manager) ResolveEntry(alias string) (channel.EntrySpec, error) {
	target, err := m.ResolveExecutionTarget(alias)
	if err != nil {
		return channel.EntrySpec{}, err
	}
	entry := target.HostEntry
	if entry.Kind != ServerKindSSH {
		return channel.EntrySpec{}, fmt.Errorf("alias %s is not an SSH server", alias)
	}
	return channel.EntrySpec{
		Alias:         entry.Alias,
		Host:          entry.Host,
		Port:          entry.Port,
		User:          entry.User,
		DefaultCWD:    entry.DefaultCWD,
		IdentityFile:  entry.Auth.IdentityFile,
		UseAgent:      entry.Auth.UseAgent,
		AllowPassword: entry.Auth.AllowPassword,
	}, nil
}

func (m *Manager) UpsertServer(entry ServerEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reg.Add(entry) // Registry에 동적으로 추가
}

func (m *Manager) IsRemoteAlias(alias string) bool {
	target, err := m.ResolveExecutionTarget(alias)
	return err == nil && target.HostAlias != LocalAlias && target.HostEntry.Kind == ServerKindSSH
}

func (m *Manager) accountAliasesForHost(hostAlias string) []string {
	hostAlias = m.ResolveAlias(hostAlias)
	if m.reg == nil {
		return nil
	}
	entries := m.reg.List()
	out := make([]string, 0)
	for _, entry := range entries {
		if entry.Kind == ServerKindAccount && m.ResolveAlias(entry.ParentAlias) == hostAlias {
			out = append(out, entry.Alias)
		}
	}
	return out
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
	target, err := m.ResolveExecutionTarget(alias)
	if err != nil {
		return err
	}
	hostAlias := target.HostAlias

	if existing := m.getSession(hostAlias); existing != nil {
		if _, _, err := existing.client.SendRequest("keepalive@openssh.com", true, nil); err == nil {
			return nil
		}
		_ = m.Disconnect(hostAlias)
	}

	session, err := newSSHSession(target.HostEntry, m.requestPassword)
	if err != nil {
		var authErr *authFailedError
		if errors.As(err, &authErr) {
			m.clearPasswordCache(hostAlias)
			session, err = newSSHSession(target.HostEntry, m.requestPassword)
		}
		if err != nil {
			return err
		}
	}

	m.mu.Lock()
	m.sessions[hostAlias] = session
	m.mu.Unlock()

	return nil
}

func (m *Manager) clearPasswordCache(alias string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pwCache, alias)
}

func (m *Manager) Disconnect(alias string) error {
	alias = m.ResolveAlias(alias)
	m.closeAccountShellsForAlias(alias)
	if alias == LocalAlias {
		return nil
	}
	target, err := m.ResolveExecutionTarget(alias)
	if err != nil {
		return err
	}
	if target.IsAccount {
		if m.chanMgr != nil {
			key := channel.ChannelKey{
				Alias:     target.HostAlias,
				Privilege: channel.PrivilegeKeyFromStack(target.HostEntry.User, laneStackForTarget(target)),
				Purpose:   channel.PurposeExec,
			}
			m.chanMgr.DropChannel(key)
		}
		m.activePrivMu.Lock()
		delete(m.activePriv, alias)
		m.activePrivMu.Unlock()
		m.mu.Lock()
		delete(m.pwCache, alias)
		delete(m.inspectCache, alias)
		m.mu.Unlock()
		return nil
	}
	hostAlias := target.HostAlias

	if m.chanMgr != nil {
		_ = m.chanMgr.DropAlias(hostAlias)
	}
	m.activePrivMu.Lock()
	delete(m.activePriv, hostAlias)
	for _, child := range m.accountAliasesForHost(hostAlias) {
		delete(m.activePriv, child)
	}
	m.activePrivMu.Unlock()

	var session *sshSession
	m.mu.Lock()
	session = m.sessions[hostAlias]
	delete(m.sessions, hostAlias)
	delete(m.pwCache, hostAlias)
	delete(m.inspectCache, hostAlias)
	for _, child := range m.accountAliasesForHost(hostAlias) {
		delete(m.pwCache, child)
		delete(m.inspectCache, child)
	}
	m.mu.Unlock()

	if session != nil {
		return session.Close()
	}
	return nil
}

func (m *Manager) DisconnectAll() error {
	m.CloseAllAccountShells()
	if m.chanMgr != nil {
		_ = m.chanMgr.CloseAll()
	}
	m.activePrivMu.Lock()
	m.activePriv = map[string]channel.PrivilegeKey{}
	m.activePrivMu.Unlock()

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
	alias, _ = m.credentialAliasAndTarget(alias, kind, "")

	m.cachePassword(alias, credentialCacheKey(kind, ""), password)

	m.mu.RLock()
	creds := m.creds
	m.mu.RUnlock()
	if creds != nil {
		_ = creds.SetPasswordForTarget(alias, kind, "", password)
	}
}

func (m *Manager) SetPasswordForTarget(alias, kind, targetUser, password string) {
	if !isValidPassword(password) {
		return
	}
	alias, targetUser = m.credentialAliasAndTarget(alias, kind, targetUser)

	m.cachePassword(alias, credentialCacheKey(kind, targetUser), password)

	m.mu.RLock()
	creds := m.creds
	m.mu.RUnlock()
	if creds != nil {
		_ = creds.SetPasswordForTarget(alias, kind, targetUser, password)
	}
}

func isValidPassword(pw string) bool {
	pw = strings.TrimSpace(pw)
	if pw == "" || len(pw) > 256 {
		return false
	}
	badChars := []string{"\x00", "\r", "\n"}
	for _, char := range badChars {
		if strings.Contains(pw, char) {
			return false
		}
	}
	return true
}

func (m *Manager) GetPassword(alias, kind string) string {
	return m.GetPasswordForTarget(alias, kind, "")
}

// GetSudoPassword implements channel.CredentialResolver. It returns the cached
// sudo/su password for alias and targetUser, falling back to the wildcard slot.
func (m *Manager) GetSudoPassword(alias, targetUser string) string {
	if pw := m.GetPasswordForTarget(alias, "sudo", targetUser); pw != "" {
		return pw
	}
	return m.GetPasswordForTarget(alias, "su", targetUser)
}

// GetLoginPassword implements channel.CredentialResolver. It returns the cached
// SSH login password for alias.
func (m *Manager) GetLoginPassword(alias string) string {
	return m.GetPassword(alias, "ssh")
}

func (m *Manager) GetPasswordForTarget(alias, kind, targetUser string) string {
	targetUser = strings.TrimSpace(targetUser)
	alias, targetUser = m.credentialAliasAndTarget(alias, kind, targetUser)
	if cached := m.cachedPassword(alias, credentialCacheKey(kind, targetUser)); cached != "" {
		return cached
	}
	if targetUser != "" {
		if cached := m.cachedPassword(alias, credentialCacheKey(kind, "")); cached != "" {
			return cached
		}
	}

	// Try credential store if available
	m.mu.RLock()
	creds := m.creds
	m.mu.RUnlock()
	if creds != nil {
		pw, ok, err := creds.GetPasswordForTarget(alias, kind, targetUser)
		if err == nil && ok && isValidPassword(pw) {
			m.cachePassword(alias, credentialCacheKey(kind, targetUser), pw)
			return pw
		}
		// Fallback to wildcard or other protocols if needed (sudo/su/ssh often share passwords)
		if kind == "su" || kind == "sudo" || kind == "ssh" {
			for _, k := range []string{"ssh", "sudo", "su"} {
				pw, ok, err := creds.GetPasswordForTarget(alias, k, targetUser)
				if err == nil && ok && isValidPassword(pw) {
					m.cachePassword(alias, credentialCacheKey(k, targetUser), pw)
					return pw
				}
				if targetUser != "" {
					pw, ok, err := creds.GetPasswordForTarget(alias, k, "")
					if err == nil && ok && isValidPassword(pw) {
						m.cachePassword(alias, credentialCacheKey(k, targetUser), pw)
						return pw
					}
				}
			}
		}
	}

	return ""
}

func (m *Manager) credentialAliasAndTarget(alias, kind, targetUser string) (string, string) {
	alias = m.ResolveAlias(alias)
	targetUser = strings.TrimSpace(targetUser)
	target, err := m.ResolveExecutionTarget(alias)
	if err != nil || !target.IsAccount {
		return alias, targetUser
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "sudo", "su":
		if targetUser == "" {
			targetUser = target.TargetUser
		}
	}
	return target.HostAlias, targetUser
}

func (m *Manager) requestPassword(alias, kind, prompt string) (string, error) {
	return m.requestPasswordForTarget(alias, kind, "", prompt)
}

func (m *Manager) requestPasswordForTarget(alias, kind, targetUser, prompt string) (string, error) {
	if pw := m.GetPasswordForTarget(alias, kind, targetUser); pw != "" {
		return pw, nil
	}

	m.mu.RLock()
	fn := m.promptFn
	m.mu.RUnlock()
	if fn == nil {
		return "", fmt.Errorf("password required for %s but no interaction allowed in this mode", alias)
	}

	promptKind := kind
	if targetUser != "" {
		promptKind = kind + "/" + targetUser
	}
	pw, err := fn(alias, promptKind, prompt)
	if err == nil && isValidPassword(pw) {
		m.SetPasswordForTarget(alias, kind, targetUser, pw)
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

func credentialCacheKey(kind, targetUser string) string {
	key := strings.TrimSpace(kind)
	if strings.TrimSpace(targetUser) != "" {
		key += "\x00" + strings.TrimSpace(targetUser)
	}
	return key
}

func privilegeTargetUser(opts ExecOptions) string {
	if strings.TrimSpace(opts.AsUser) != "" {
		return strings.TrimSpace(opts.AsUser)
	}
	switch strings.ToLower(strings.TrimSpace(opts.PrivilegeMethod)) {
	case PrivilegeSudo, PrivilegeSU:
		return "root"
	default:
		return ""
	}
}

func applyAccountTargetOptions(target ExecutionTarget, opts ExecOptions) ExecOptions {
	if !target.IsAccount {
		return opts
	}
	if strings.TrimSpace(opts.AsUser) == "" {
		opts.AsUser = target.TargetUser
	}
	if strings.TrimSpace(opts.PrivilegeMethod) == "" {
		opts.PrivilegeMethod = target.SwitchMethod
	}
	if strings.TrimSpace(opts.ReuseSession) == "" {
		opts.ReuseSession = ReuseSessionRequired
	}
	if strings.TrimSpace(opts.WorkingDir) == "" {
		opts.WorkingDir = target.DefaultCWD
	}
	return opts
}

func (m *Manager) Exec(ctx context.Context, alias, command string, subQuery ExecuteSubQueryFunc) (ExecResult, error) {
	return m.ExecWithOptions(ctx, alias, command, ExecOptions{}, subQuery)
}

func (m *Manager) StartExecWithOptions(ctx context.Context, alias, command string, opts ExecOptions, subQuery ExecuteSubQueryFunc) (*ExecHandle, error) {
	alias = m.ResolveAlias(alias)
	target, targetErr := m.ResolveExecutionTarget(alias)
	credentialAlias := alias
	if targetErr == nil {
		credentialAlias = target.HostAlias
		if target.IsAccount {
			opts = applyAccountTargetOptions(target, opts)
		}
	}
	if opts.Password != "" {
		m.SetPassword(credentialAlias, "ssh", opts.Password)
	}
	if opts.RootPassword != "" {
		m.SetPasswordForTarget(credentialAlias, "sudo", privilegeTargetUser(opts), opts.RootPassword)
		m.SetPasswordForTarget(credentialAlias, "su", privilegeTargetUser(opts), opts.RootPassword)
		m.SetPassword(credentialAlias, "sudo", opts.RootPassword)
	}
	if m.shouldRouteViaChannel(alias, opts) {
		handle, err := m.startExecViaChannel(ctx, alias, command, opts)
		if err == nil {
			return handle, nil
		}
		// ElevationRejectedError is authoritative — do not fall through.
		var elevErr *ElevationRejectedError
		if errors.As(err, &elevErr) {
			return nil, err
		}
		// Other channel errors fall through to legacy paths (priming failure,
		// non-bash login shell, etc.).
	}
	// Provide a fallback so ssh_session can request a new sudo password via
	// the TUI modal when the cached credential is rejected.
	if opts.RequestSudoPassword == nil {
		capturedAlias := credentialAlias
		capturedTarget := privilegeTargetUser(opts)
		opts.RequestSudoPassword = func(prompt string) (string, error) {
			pw, err := m.requestPasswordForTarget(capturedAlias, "sudo", capturedTarget, prompt)
			if err == nil && pw != "" {
				opts.RootPassword = pw
			}
			return pw, err
		}
	}
	if shouldUseAccountShell(opts) {
		account, err := m.getOrCreateAccountShell(ctx, alias, opts)
		if err != nil {
			return nil, err
		}
		return account.Start(ctx, command, opts, subQuery), nil
	}

	if !m.IsRemoteAlias(alias) {
		return m.startLocalExecStreaming(ctx, command, opts)
	}

	hostAlias := alias
	if targetErr == nil {
		hostAlias = target.HostAlias
	}
	session, err := m.ensureSession(hostAlias)
	if err != nil {
		return nil, err
	}
	return session.StartExec(ctx, command, opts, subQuery)
}

func (m *Manager) ExecWithOptions(ctx context.Context, alias, command string, opts ExecOptions, subQuery ExecuteSubQueryFunc) (ExecResult, error) {
	alias = m.ResolveAlias(alias)
	target, targetErr := m.ResolveExecutionTarget(alias)
	credentialAlias := alias
	if targetErr == nil {
		credentialAlias = target.HostAlias
		if target.IsAccount {
			opts = applyAccountTargetOptions(target, opts)
		}
	}
	if opts.Password != "" {
		m.SetPassword(credentialAlias, "ssh", opts.Password)
	}
	if opts.RootPassword != "" {
		m.SetPasswordForTarget(credentialAlias, "sudo", privilegeTargetUser(opts), opts.RootPassword)
		m.SetPasswordForTarget(credentialAlias, "su", privilegeTargetUser(opts), opts.RootPassword)
		m.SetPassword(credentialAlias, "sudo", opts.RootPassword)
	}
	if m.shouldRouteViaChannel(alias, opts) {
		res, err := m.execViaChannel(ctx, alias, command, opts)
		if err == nil {
			return res, nil
		}
		// Fall through if the channel backbone declined the request.
	}
	if opts.RequestSudoPassword == nil {
		capturedAlias := credentialAlias
		opts.RequestSudoPassword = func(prompt string) (string, error) {
			pw, err := m.requestPassword(capturedAlias, "sudo", prompt)
			if err == nil && pw != "" {
				opts.RootPassword = pw
			}
			return pw, err
		}
	}
	if shouldUseAccountShell(opts) {
		account, err := m.getOrCreateAccountShell(ctx, alias, opts)
		if err != nil {
			return ExecResult{}, err
		}
		return account.Run(ctx, command, opts, subQuery)
	}

	if !m.IsRemoteAlias(alias) {
		return m.execLocal(ctx, command, opts)
	}

	hostAlias := alias
	if targetErr == nil {
		hostAlias = target.HostAlias
	}
	session, err := m.ensureSession(hostAlias)
	if err != nil {
		return ExecResult{}, err
	}
	return session.Exec(ctx, command, opts, subQuery)
}

func (m *Manager) CurrentUser(alias string) string {
	target, err := m.ResolveExecutionTarget(alias)
	if err == nil && target.IsAccount {
		return target.TargetUser
	}
	if !m.IsRemoteAlias(alias) {
		if user := os.Getenv("USER"); user != "" {
			return user
		}
		return os.Getenv("USERNAME")
	}
	hostAlias := alias
	if err == nil {
		hostAlias = target.HostAlias
	}
	session := m.getSession(hostAlias)
	if session == nil {
		return ""
	}
	return session.entry.User
}

func (m *Manager) ListDir(ctx context.Context, alias, root string, recursive bool, depth int) ([]FileEntry, error) {
	alias = m.ResolveAlias(alias)
	hostAlias := alias
	if target, err := m.ResolveExecutionTarget(alias); err == nil {
		hostAlias = target.HostAlias
	}
	if useChannelBackbone() && m.IsRemoteAlias(alias) {
		ch, err := m.ChannelManager().AcquireSFTP(ctx, hostAlias)
		if err == nil {
			raw, err := ch.ListDir(root, recursive, depth)
			if err == nil {
				return convertChannelFileEntries(raw), nil
			}
		}
	}
	if hostAlias == LocalAlias {
		base := strings.TrimSpace(root)
		if base == "" {
			base = "."
		}
		base = filepath.Clean(base)
		entries := make([]FileEntry, 0, 64)
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == base {
				return nil
			}
			rel, relErr := filepath.Rel(base, path)
			if relErr != nil {
				return relErr
			}
			relDepth := strings.Count(filepath.ToSlash(rel), "/") + 1
			if !recursive && relDepth > 1 {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if depth > 0 && relDepth >= depth {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			info, infoErr := d.Info()
			if infoErr != nil {
				return infoErr
			}
			entries = append(entries, FileEntry{
				Name:    d.Name(),
				Path:    path,
				IsDir:   d.IsDir(),
				Size:    info.Size(),
				Mode:    info.Mode().String(),
				ModTime: info.ModTime(),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
		return entries, nil
	}
	session, err := m.ensureSession(hostAlias)
	if err != nil {
		return nil, err
	}
	return session.listDirSFTP(root, recursive, depth)
}

func (m *Manager) ReadFile(ctx context.Context, alias, path string) ([]byte, error) {
	alias = m.ResolveAlias(alias)
	hostAlias := alias
	if target, err := m.ResolveExecutionTarget(alias); err == nil {
		hostAlias = target.HostAlias
	}
	if hostAlias == LocalAlias {
		return os.ReadFile(path)
	}
	if useChannelBackbone() {
		ch, err := m.ChannelManager().AcquireSFTP(ctx, hostAlias)
		if err == nil {
			if data, err := ch.ReadFile(path); err == nil {
				return data, nil
			}
		}
	}
	session, err := m.ensureSession(hostAlias)
	if err != nil {
		return nil, err
	}
	return session.readFileSFTP(path)
}

func (m *Manager) WriteFile(ctx context.Context, alias, path string, content []byte, overwrite bool) error {
	alias = m.ResolveAlias(alias)
	hostAlias := alias
	if target, err := m.ResolveExecutionTarget(alias); err == nil {
		hostAlias = target.HostAlias
	}
	if !m.IsRemoteAlias(alias) {
		return os.WriteFile(path, content, 0o644)
	}
	if useChannelBackbone() {
		ch, err := m.ChannelManager().AcquireSFTP(ctx, hostAlias)
		if err == nil {
			if err := ch.WriteFile(path, content, overwrite); err == nil {
				return nil
			}
		}
	}
	session, err := m.ensureSession(hostAlias)
	if err != nil {
		return err
	}
	return session.writeFileSFTP(path, content, overwrite)
}

func (m *Manager) CopyFile(ctx context.Context, srcAlias, srcPath, dstAlias, dstPath string, overwrite bool) error {
	return m.CopyFileWithProgress(ctx, srcAlias, srcPath, dstAlias, dstPath, overwrite, nil)
}

func (m *Manager) CopyFileWithProgress(ctx context.Context, srcAlias, srcPath, dstAlias, dstPath string, overwrite bool, progress func(int64)) error {
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

	if progress == nil {
		_, err = io.Copy(dst, src)
		return err
	}

	// Wrap dst with a progress tracker
	pw := &progressWriter{
		Writer:  dst,
		onWrite: progress,
	}
	_, err = io.Copy(pw, src)
	return err
}

type progressWriter struct {
	io.Writer
	total   int64
	onWrite func(int64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.Writer.Write(p)
	if n > 0 {
		pw.total += int64(n)
		pw.onWrite(pw.total)
	}
	return n, err
}

func (m *Manager) openReader(alias, path string) (io.ReadCloser, error) {
	alias = m.ResolveAlias(alias)
	hostAlias := alias
	if target, err := m.ResolveExecutionTarget(alias); err == nil {
		hostAlias = target.HostAlias
	}
	if hostAlias == LocalAlias {
		return os.Open(path)
	}
	if useChannelBackbone() {
		ch, err := m.ChannelManager().AcquireSFTP(context.Background(), hostAlias)
		if err == nil {
			if rc, err := ch.OpenReader(path); err == nil {
				return rc, nil
			}
		}
	}
	session, err := m.ensureSession(hostAlias)
	if err != nil {
		return nil, err
	}
	return session.openFileForReadSFTP(path)
}

func (m *Manager) openWriter(alias, path string, overwrite bool) (io.WriteCloser, error) {
	alias = m.ResolveAlias(alias)
	hostAlias := alias
	if target, err := m.ResolveExecutionTarget(alias); err == nil {
		hostAlias = target.HostAlias
	}
	if hostAlias == LocalAlias {
		dir := filepath.Dir(path)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
		flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
		if !overwrite {
			flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
		}
		return os.OpenFile(path, flags, 0o644)
	}
	if useChannelBackbone() {
		ch, err := m.ChannelManager().AcquireSFTP(context.Background(), hostAlias)
		if err == nil {
			if wc, err := ch.OpenWriter(path, overwrite); err == nil {
				return wc, nil
			}
		}
	}
	session, err := m.ensureSession(hostAlias)
	if err != nil {
		return nil, err
	}
	return session.openFileForWriteSFTP(path, overwrite)
}

func (m *Manager) OpenTunnel(ctx context.Context, alias, localAddr, remoteAddr string) (TunnelInfo, error) {
	alias = m.ResolveAlias(alias)
	hostAlias := alias
	if target, err := m.ResolveExecutionTarget(alias); err == nil {
		hostAlias = target.HostAlias
	}
	if useChannelBackbone() && m.IsRemoteAlias(alias) {
		ch, err := m.ChannelManager().AcquireTunnel(ctx, hostAlias)
		if err == nil {
			info, err := ch.Open(localAddr, remoteAddr)
			if err == nil {
				return convertChannelTunnel(info), nil
			}
		}
	}
	session, err := m.ensureSession(hostAlias)
	if err != nil {
		return TunnelInfo{}, err
	}
	return session.openTunnel(localAddr, remoteAddr)
}

func (m *Manager) CloseTunnel(alias, tunnelID string) error {
	alias = m.ResolveAlias(alias)
	hostAlias := alias
	if target, err := m.ResolveExecutionTarget(alias); err == nil {
		hostAlias = target.HostAlias
	}
	if useChannelBackbone() && m.IsRemoteAlias(alias) {
		ch, err := m.ChannelManager().AcquireTunnel(context.Background(), hostAlias)
		if err == nil {
			if err := ch.CloseTunnel(tunnelID); err == nil {
				return nil
			}
		}
	}
	session := m.getSession(hostAlias)
	if session == nil {
		return fmt.Errorf("session not found")
	}
	return session.closeTunnel(tunnelID)
}

func (m *Manager) ListTunnels(alias string) ([]TunnelInfo, error) {
	alias = m.ResolveAlias(alias)
	hostAlias := alias
	if target, err := m.ResolveExecutionTarget(alias); err == nil {
		hostAlias = target.HostAlias
	}
	if useChannelBackbone() && m.IsRemoteAlias(alias) {
		ch, err := m.ChannelManager().AcquireTunnel(context.Background(), hostAlias)
		if err == nil {
			raw := ch.List()
			out := make([]TunnelInfo, 0, len(raw))
			for _, t := range raw {
				out = append(out, convertChannelTunnel(t))
			}
			if len(out) > 0 {
				return out, nil
			}
		}
	}
	session := m.getSession(hostAlias)
	if session == nil {
		return nil, fmt.Errorf("session not found")
	}
	return session.listTunnels(), nil
}

func (m *Manager) MetricsSnapshot(ctx context.Context, alias string) (MetricsSnapshot, error) {
	alias = m.ResolveAlias(alias)
	hostAlias := alias
	if target, err := m.ResolveExecutionTarget(alias); err == nil {
		hostAlias = target.HostAlias
	}
	if !m.IsRemoteAlias(alias) {
		return MetricsSnapshot{Alias: alias, CollectedAt: time.Now()}, nil
	}
	cm := m.ChannelManager()
	ch, err := cm.AcquireMetrics(ctx, hostAlias)
	if err != nil {
		return MetricsSnapshot{Alias: alias, CollectedAt: time.Now(), Errors: map[string]string{"acquire": err.Error()}}, err
	}
	raw, err := ch.Collect(ctx)
	if err != nil {
		return MetricsSnapshot{Alias: alias, CollectedAt: time.Now(), Errors: map[string]string{"collect": err.Error()}}, err
	}
	snap := MetricsSnapshot{
		Alias:       alias,
		CollectedAt: time.Now(),
		Load:        raw.LoadAvg,
		Memory:      raw.MemInfo,
		Disk:        raw.DiskInfo,
		Uptime:      raw.UptimeRaw,
		Processes:   raw.Processes,
		GPU:         raw.GPU,
		Errors:      raw.Errors,
	}
	if snap.Errors == nil {
		snap.Errors = map[string]string{}
	}
	return snap, nil
}

func (m *Manager) Status() []StatusEntry {
	m.mu.RLock()
	active := m.active
	m.mu.RUnlock()

	entries := m.reg.List()
	var res []StatusEntry

	// Add local workspace first
	res = append(res, StatusEntry{
		Alias:     LocalAlias,
		Kind:      ServerKindLocal,
		Connected: true,
		User:      firstNonEmpty(os.Getenv("USER"), os.Getenv("USERNAME")),
		Active:    active == "" || active == LocalAlias,
	})

	for _, e := range entries {
		if e.Alias == LocalAlias {
			continue // Already added manually above
		}
		hostAlias := e.Alias
		if e.Kind == ServerKindAccount {
			hostAlias = e.ParentAlias
		}
		session := m.getSession(hostAlias)
		connected := session != nil
		if !connected && m.chanPool != nil && m.chanPool.Lookup(hostAlias) != nil {
			connected = true
		}
		user := e.User
		if e.Kind == ServerKindSSH && connected && session != nil && session.entry.User != "" {
			user = session.entry.User
		}

		res = append(res, StatusEntry{
			Alias:       e.Alias,
			Kind:        e.Kind,
			ParentAlias: e.ParentAlias,
			Connected:   connected,
			User:        user,
			TargetUser:  e.User,
			Active:      strings.EqualFold(active, e.Alias),
		})
	}
	return res
}

func (m *Manager) ensureSession(alias string) (*sshSession, error) {
	hostAlias := alias
	if target, err := m.ResolveExecutionTarget(alias); err == nil {
		hostAlias = target.HostAlias
	}
	if err := m.Connect(hostAlias); err != nil {
		return nil, err
	}
	session := m.getSession(hostAlias)
	if session == nil {
		return nil, fmt.Errorf("ssh session unavailable for %s", hostAlias)
	}
	return session, nil
}

func (m *Manager) getSession(alias string) *sshSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[alias]
}

func (m *Manager) getOrCreateAccountShell(ctx context.Context, alias string, opts ExecOptions) (*accountShellSession, error) {
	alias = m.ResolveAlias(alias)
	target, targetErr := m.ResolveExecutionTarget(alias)
	hostAlias := alias
	if targetErr == nil {
		hostAlias = target.HostAlias
		if target.IsAccount {
			opts = applyAccountTargetOptions(target, opts)
		}
	}
	key := accountShellKey(alias, opts)

	m.mu.RLock()
	if existing := m.accountShells[key]; existing != nil && !existing.closedFlag.Load() {
		m.mu.RUnlock()
		return existing, nil
	}
	m.mu.RUnlock()

	var account *accountShellSession
	var err error
	if m.IsRemoteAlias(alias) {
		session, sessionErr := m.ensureSession(hostAlias)
		if sessionErr != nil {
			return nil, sessionErr
		}
		account, err = session.openAccountShell(ctx, opts)
	} else {
		account, err = m.openLocalAccountShell(ctx, opts)
	}
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	if existing := m.accountShells[key]; existing != nil && !existing.closedFlag.Load() {
		m.mu.Unlock()
		_ = account.Close()
		return existing, nil
	}
	m.accountShells[key] = account
	m.mu.Unlock()
	return account, nil
}

func (m *Manager) ListAccountShells(alias string) []AccountShellInfo {
	alias = m.ResolveAlias(alias)
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]AccountShellInfo, 0, len(m.accountShells))
	for _, account := range m.accountShells {
		if account == nil {
			continue
		}
		info := account.Info()
		if alias == "" || alias == info.Alias {
			out = append(out, info)
		}
	}
	return out
}

func (m *Manager) CloseAccountShell(alias, sessionID string) error {
	alias = m.ResolveAlias(alias)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("account shell session id is required")
	}

	var account *accountShellSession
	m.mu.Lock()
	for k, candidate := range m.accountShells {
		if candidate == nil {
			continue
		}
		info := candidate.Info()
		if info.Alias == alias && info.ID == sessionID {
			account = candidate
			delete(m.accountShells, k)
			break
		}
	}
	m.mu.Unlock()
	if account == nil {
		return fmt.Errorf("account shell not found: %s", sessionID)
	}
	return account.Close()
}

func (m *Manager) CloseAllAccountShells() {
	m.mu.Lock()
	accounts := make([]*accountShellSession, 0, len(m.accountShells))
	for _, account := range m.accountShells {
		if account != nil {
			accounts = append(accounts, account)
		}
	}
	m.accountShells = make(map[string]*accountShellSession)
	m.mu.Unlock()

	for _, account := range accounts {
		_ = account.Close()
	}
}

func (m *Manager) closeAccountShellsForAlias(alias string) {
	m.mu.Lock()
	accounts := make([]*accountShellSession, 0)
	for key, account := range m.accountShells {
		if account == nil {
			continue
		}
		if account.alias == alias {
			accounts = append(accounts, account)
			delete(m.accountShells, key)
		}
	}
	m.mu.Unlock()
	for _, account := range accounts {
		_ = account.Close()
	}
}
