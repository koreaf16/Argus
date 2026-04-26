// Package connector
// File: activation_capabilities.go
// Description: capability inference for activation resolver
// Responsibility: infer usable command capabilities from service type + evidence

package connector

import (
	"context"
	"strings"

	"github.com/yourorg/infractl/internal/executor"
)

func (r *ActivationResolver) probeCapabilities(
	_ context.Context,
	_ executor.Executor,
	serviceType string,
	_ ActivationRequest,
	evidence []ActivationEvidence,
) map[string]bool {
	caps := map[string]bool{
		"status": true,
	}

	switch strings.ToLower(strings.TrimSpace(serviceType)) {
	case "oracle":
		caps["query"] = true
		caps["sessions"] = true
		caps["locks"] = true
		caps["top_sql"] = true
		caps["alert_log"] = true
		if hasCapability(evidence, "tkprof") || hasCapability(evidence, "trace") {
			caps["trace_profile"] = true
		}
	case "mysql":
		caps["query"] = true
		caps["processlist"] = true
		caps["databases"] = true
	case "postgresql", "postgres":
		caps["query"] = true
		caps["connections"] = true
		caps["locks"] = true
		caps["databases"] = true
	case "tomcat", "weblogic":
		caps["logs"] = true
		caps["restart"] = true
	default:
		caps["logs"] = true
	}

	return caps
}
