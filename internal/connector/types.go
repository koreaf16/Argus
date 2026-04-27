package connector

type ConnectorSpec struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Source      string            `json:"source"`
	SourceURL   string            `json:"source_url,omitempty"`
	Runtime     RuntimeType       `json:"runtime"`
	Command     string            `json:"command"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	EnvPrompts  []EnvPrompt       `json:"env_prompts,omitempty"`
	Category    string            `json:"category,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Version     string            `json:"version,omitempty"`
	InstallDir  string            `json:"install_dir,omitempty"`
}

type RuntimeType string

const (
	RuntimeNPX    RuntimeType = "npx"
	RuntimeUVX    RuntimeType = "uvx"
	RuntimeGo     RuntimeType = "go"
	RuntimeDocker RuntimeType = "docker"
	RuntimeBinary RuntimeType = "binary"
	RuntimeCustom RuntimeType = "custom"
)

type EnvPrompt struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Secret      bool   `json:"secret"`
	Default     string `json:"default,omitempty"`
}

type InstalledConnector struct {
	Spec   ConnectorSpec `json:"spec"`
	Status string        `json:"status"` // active, inactive
}
