// Package probes contains individual system-inspection probes used by the
// inventory runner. Each probe contributes a bash script fragment that is
// assembled by the runner into a single batched SSH call, then receives its
// section of the output to parse.
package probes

// Probe contributes a bash fragment and parses its output section.
// Probes must be safe to run as the SSH login user (no sudo).
type Probe interface {
	Name() string
	ScriptFragment() string
	Parse(stdout string) (Result, error)
}

// Result is a sum-type returned by Probe.Parse. Only one field is set.
type Result struct {
	Docker     []DockerContainer
	Kubernetes *K8sResult
	LLMServing []LLMServingResult
}

// DockerContainer is one line of `docker ps -a --format json` output.
type DockerContainer struct {
	ID      string
	Name    string
	Image   string
	State   string
	Ports   string
	Command string
}

// K8sResult holds parsed kubectl output.
type K8sResult struct {
	Context    string
	Permission string
	Nodes      []string
	Namespaces []string
	Pods       []K8sPod
	Services   []K8sService
}

// K8sPod is one row from `kubectl get pods -A`.
type K8sPod struct {
	Namespace   string
	Name        string
	Phase       string
	Image       string
	NodeName    string
	ContainerID string
}

// K8sService is one row from `kubectl get svc -A`.
type K8sService struct {
	Namespace string
	Name      string
	Type      string
	Port      string
}

// LLMServingResult describes one detected LLM serving engine and its models.
type LLMServingResult struct {
	Engine    string
	Port      int
	Models    []string
	CgroupPid int
	Cgroup    string
}
