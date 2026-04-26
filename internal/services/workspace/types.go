package workspace

import "time"

type ServerKind string

const (
	ServerKindLocal ServerKind = "local"
	ServerKindSSH   ServerKind = "ssh"
)

type ServerAuth struct {
	IdentityFile  string `json:"identity_file,omitempty"`
	UseAgent      bool   `json:"use_agent,omitempty"`
	AllowPassword bool   `json:"allow_password,omitempty"`
}

type ServerEntry struct {
	Alias      string     `json:"alias"`
	Kind       ServerKind `json:"kind"`
	Host       string     `json:"host,omitempty"`
	Port       int        `json:"port,omitempty"`
	User       string     `json:"user,omitempty"`
	DefaultCWD string     `json:"default_cwd,omitempty"`
	Auth       ServerAuth `json:"auth,omitempty"`
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
}

type StatusEntry struct {
	Alias      string
	Kind       ServerKind
	Connected  bool
	CurrentCWD string
	User       string
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
