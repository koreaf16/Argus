package workspace

import "time"

type ServerKind string

const (
	ServerKindLocal ServerKind = "local"
	ServerKindSSH   ServerKind = "ssh"
	// ServerKindAccount is a logical work target reached through a parent SSH
	// host, usually by entering `su - <user>` from the host login account.
	ServerKindAccount ServerKind = "account"
)

type ServerAuth struct {
	IdentityFile  string `json:"identity_file,omitempty"`
	UseAgent      bool   `json:"use_agent,omitempty"`
	AllowPassword bool   `json:"allow_password,omitempty"`
}

// Elevation specifies whether and how this server permits privilege escalation
// (sudo / su). When Allowed is false every Enter/Inline transition is refused
// before a channel is acquired; the LLM is informed via the workspace prompt
// so it can stop proactively rather than waiting for the channel-layer reject.
type Elevation struct {
	// Allowed gates the entire feature. False means all sudo/su commands are
	// refused for this alias.
	Allowed bool `json:"allowed,omitempty"`

	// Mode selects how the sudo/su password is sourced when Allowed=true.
	//   "none"        — explicit refusal (same as Allowed=false but preserves config)
	//   "password"    — use the credential cached under kind="sudo"/"su"
	//   "reuse_login" — forward the SSH login password as the sudo password
	Mode string `json:"mode,omitempty"`

	// TargetUsers restricts elevation to these accounts. Empty means any target
	// is allowed. Values are trimmed and deduplicated on save.
	TargetUsers []string `json:"target_users,omitempty"`
}

type ServerEntry struct {
	Alias       string     `json:"alias"`
	Kind        ServerKind `json:"kind"`
	Host        string     `json:"host,omitempty"`
	Port        int        `json:"port,omitempty"`
	User        string     `json:"user,omitempty"`
	ParentAlias string     `json:"parent_alias,omitempty"`
	SwitchMethod string     `json:"switch_method,omitempty"`
	OSType      string     `json:"os_type,omitempty"` // "linux", "windows", etc.
	DefaultCWD  string     `json:"default_cwd,omitempty"`
	Auth        ServerAuth `json:"auth,omitempty"`
	Elevation   Elevation  `json:"elevation,omitempty"`
	IsEphemeral bool       `json:"is_ephemeral,omitempty"`
}

type EndpointPath struct {
	Alias    string `json:"alias"`
	RawPath  string `json:"raw_path"`
	OSType   string `json:"os_type"`
	IsRemote bool   `json:"is_remote"`
}

type Catalog struct {
	Active  string        `json:"active,omitempty"`
	Servers []ServerEntry `json:"servers,omitempty"`
}

type ExecResult struct {
	Stdout string
	Stderr string
	Code   int
	CWD    string
	User   string
}

type ExecOptions struct {
	WorkingDir string
	// Shell may be "", "bash", or "powershell".
	Shell string
	// ChunkCallback streams stdout chunks while the command is running.
	ChunkCallback func(string)
	// Optional passwords for session priming
	Password     string
	RootPassword string
	// AsUser requests an account-scoped interactive shell. When empty, commands
	// run as the connected account unless ReuseSession is "required".
	AsUser string
	// PrivilegeMethod may be "none", "sudo", or "su".
	PrivilegeMethod string
	// ReuseSession may be "auto", "never", or "required".
	ReuseSession string
	// SessionID selects a named persistent account shell.
	SessionID string
	// PTYMode may be "auto", "always", or "never". Account shells require PTY.
	PTYMode string
	// Role/Channel identify the workflow context that requested this execution.
	Role    string
	Channel string
	// RequestSudoPassword is called when the cached RootPassword is wrong and
	// the remote side prompts for sudo again. Set automatically by Manager.
	// Returning ("", err) leaves the command blocked; the caller should cancel ctx.
	RequestSudoPassword func(prompt string) (string, error)
}

type AccountShellInfo struct {
	ID              string `json:"id"`
	Alias           string `json:"alias"`
	User            string `json:"user"`
	Shell           string `json:"shell"`
	PrivilegeMethod string `json:"privilege_method"`
	Role            string `json:"role,omitempty"`
	Channel         string `json:"channel,omitempty"`
	CWD             string `json:"cwd"`
	Active          bool   `json:"active"`
}

type StatusEntry struct {
	Alias      string
	Kind       ServerKind
	ParentAlias string
	Connected  bool
	CurrentCWD string
	User       string
	TargetUser string
	Active     bool
}

type TunnelInfo struct {
	ID         string
	Alias      string
	LocalAddr  string
	RemoteAddr string
	Active     bool
}

type MetricsSnapshot struct {
	Alias       string
	CollectedAt time.Time
	Load        string
	CPU         string
	Memory      string
	Disk        string
	Uptime      string
	Processes   string
	GPU         string
	Errors      map[string]string
}

// InspectSnapshot is the result of server_inspect.
type InspectSnapshot struct {
	Alias       string
	CollectedAt time.Time
	OS          string
	Kernel      string
	Shell       string
	User        string
	CWD         string
	Uptime      string
	Memory      string
	Disk        string
	Listeners   string
	Services    string
	Processes   string
	Docker      string
	Errors      map[string]string
}

type FileEntry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"mod_time"`
}

type ExecHandle struct {
	Stream <-chan string
	Result <-chan ExecResult
	Write  func(string) error
	Kill   func()
}
