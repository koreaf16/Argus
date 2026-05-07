// Package probes contains individual system-inspection probes used by the
// inventory runner. Each probe runs a bash script on the target machine and
// parses the output into a Result.
package probes

import (
	"context"
	"time"
)

// Probe describes a system inspection probe.
// Probes must be safe to run as the SSH login user (no sudo).
type Probe interface {
	Name() string
	PreferredTimeout() time.Duration
	Run(ctx context.Context, fn ProbeExec) (Result, error)
}

// Result is a sum-type returned by Probe.Run. Only one field is typically set.
type Result struct {
	Docker       []DockerContainer
	Kubernetes   *K8sResult
	LLMServing   []LLMServingResult
	SystemHeader *SystemHeaderResult
	Databases    []DatabaseResult
	WAS          []WASResult
	MQ           []MQResult
	GPU          *GPUResult
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

// DatabaseResult is a raw probe result for a database engine.
type DatabaseResult struct {
	Engine     string
	Version    string
	Instances  []string
	Listeners  []string
	Confidence string
	Evidence   []string
}

// WASResult is a raw probe result for a web application server.
type WASResult struct {
	Engine     string
	Version    string
	Home       string
	Ports      []int
	Confidence string
	Evidence   []string
}

// MQResult is a raw probe result for a message queue broker.
type MQResult struct {
	Engine     string
	Version    string
	Ports      []int
	Confidence string
	Evidence   []string
}

// GPUDevice is a single GPU device from nvidia-smi.
type GPUDevice struct {
	Name          string
	DriverVersion string
	MemoryMB      int
}

// GPUResult holds NVIDIA GPU information.
type GPUResult struct {
	Devices    []GPUDevice
	CUDADriver string
}
