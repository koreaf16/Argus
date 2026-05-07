package inventory

import "time"

// Status describes the collection state of an InventorySnapshot.
type Status string

const (
	StatusPending Status = "pending"
	StatusReady   Status = "ready"
	StatusPartial Status = "partial"
	StatusFailed  Status = "failed"
)

// InventorySnapshot is the result of a full inventory scan for one server alias.
type InventorySnapshot struct {
	Alias       string
	CollectedAt time.Time
	DurationMs  int64
	Status      Status

	Containers []ContainerInfo
	Kubernetes *K8sInfo
	LLMServing []LLMServingInfo

	Errors     map[string]string // probe name → error message
	ArtifactID string
}

// ContainerInfo describes a single Docker/Podman container.
type ContainerInfo struct {
	ID      string
	Name    string
	Image   string
	State   string // "running" | "exited" | ...
	Ports   string
	Command string
}

// K8sInfo holds cluster-level Kubernetes information.
type K8sInfo struct {
	Context    string
	Permission string // "admin" | "limited" | "unknown"
	NodeCount  int
	PodCount   int
	Namespaces []string
	Workloads  []K8sWorkload
}

// K8sWorkload is a single pod observed via kubectl.
type K8sWorkload struct {
	Namespace   string
	Name        string
	Phase       string // "Running" | "Pending" | ...
	Image       string
	NodeName    string
	ContainerID string // docker/containerd container ID prefix
}

// LLMServingInfo describes one LLM serving process.
type LLMServingInfo struct {
	Engine   string     // "vllm" | "ollama" | "sglang" | "tgi"
	Models   []LLMModel // models served by this process
	Port     int
	Endpoint string
	Hosting  LLMHosting
}

// LLMModel is a single model entry reported by an LLM serving engine.
type LLMModel struct {
	ID     string // e.g. "google/gemma-2-27b-it"
	Family string // e.g. "gemma" (derived from ID)
}

// LLMHosting describes how an LLM engine is running on the host.
type LLMHosting struct {
	Type      string // "k8s" | "docker" | "native" | "systemd" | "unknown"
	K8sPod    string // "namespace/pod-name"
	Container string // docker container name
	Pid       int
}
