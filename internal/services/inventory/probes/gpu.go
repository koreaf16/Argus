package probes

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// GPUProbe detects NVIDIA GPU devices via nvidia-smi.
type GPUProbe struct{}

func (p *GPUProbe) Name() string { return "gpu" }

func (p *GPUProbe) PreferredTimeout() time.Duration { return 5 * time.Second }

func (p *GPUProbe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, p.ScriptFragment())
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

func (p *GPUProbe) ScriptFragment() string {
	return `set +e
printf '<<GPU:smi>>\n'
command -v nvidia-smi >/dev/null 2>&1 && \
  nvidia-smi --query-gpu=name,driver_version,memory.total --format=csv,noheader 2>/dev/null | head -16
printf '<<GPU:driver>>\n'
cat /proc/driver/nvidia/version 2>/dev/null | head -2
printf '<<GPU:end>>\n'`
}

func (p *GPUProbe) Parse(stdout string) (Result, error) {
	sections := splitTagSections(stdout, "GPU")
	smi := strings.TrimSpace(sections["smi"])
	driver := strings.TrimSpace(sections["driver"])

	if smi == "" && driver == "" {
		return Result{}, nil
	}

	res := &GPUResult{}

	// Parse nvidia-smi CSV output: "NVIDIA A100-SXM4-80GB, 525.105.17, 81920 MiB"
	for _, line := range strings.Split(smi, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ", ", 3)
		if len(parts) < 2 {
			continue
		}
		dev := GPUDevice{
			Name:          strings.TrimSpace(parts[0]),
			DriverVersion: strings.TrimSpace(parts[1]),
		}
		if len(parts) >= 3 {
			memStr := strings.TrimSpace(strings.TrimSuffix(parts[2], " MiB"))
			if mb, err := strconv.Atoi(memStr); err == nil {
				dev.MemoryMB = mb
			}
		}
		res.Devices = append(res.Devices, dev)
	}

	// Extract CUDA driver version from /proc/driver/nvidia/version.
	for _, line := range strings.Split(driver, "\n") {
		if strings.Contains(line, "NVRM version") {
			fields := strings.Fields(line)
			// "NVRM version: NVIDIA UNIX x86_64 Kernel Module  525.105.17  ..."
			for i, f := range fields {
				if f == "Module" && i+1 < len(fields) {
					res.CUDADriver = fields[i+1]
					break
				}
			}
		}
	}

	if len(res.Devices) == 0 && res.CUDADriver == "" {
		return Result{}, nil
	}
	return Result{GPU: res}, nil
}
