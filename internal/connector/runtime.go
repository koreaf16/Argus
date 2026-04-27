package connector

import (
	"fmt"
	"os/exec"
)

func CheckRuntime(rt RuntimeType) error {
	switch rt {
	case RuntimeNPX:
		if _, err := exec.LookPath("npx"); err != nil {
			return fmt.Errorf("npx is not installed (Node.js is required)")
		}
	case RuntimeUVX:
		if _, err := exec.LookPath("uvx"); err != nil {
			return fmt.Errorf("uvx is not installed (Astral uv is required)")
		}
	case RuntimeGo:
		if _, err := exec.LookPath("go"); err != nil {
			return fmt.Errorf("go is not installed")
		}
	case RuntimeDocker:
		if _, err := exec.LookPath("docker"); err != nil {
			return fmt.Errorf("docker is not installed")
		}
	}
	return nil
}

func ResolveCommand(spec ConnectorSpec) (string, []string, error) {
	if err := CheckRuntime(spec.Runtime); err != nil {
		return "", nil, err
	}
	
	// Could implement platform-specific command resolutions here
	// e.g. .cmd extensions on Windows. For now, rely on standard PATH and mcp.Manager logic.
	
	return spec.Command, spec.Args, nil
}
