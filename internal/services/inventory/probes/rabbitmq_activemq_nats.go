package probes

import (
	"context"
	"strings"
	"time"
)

// RabbitmqActivemqNatsProbe detects RabbitMQ, ActiveMQ, and NATS brokers.
type RabbitmqActivemqNatsProbe struct{}

func (p *RabbitmqActivemqNatsProbe) Name() string { return "rabbitmq_activemq_nats" }

func (p *RabbitmqActivemqNatsProbe) PreferredTimeout() time.Duration { return 3 * time.Second }

func (p *RabbitmqActivemqNatsProbe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, p.ScriptFragment())
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

func (p *RabbitmqActivemqNatsProbe) ScriptFragment() string {
	return `set +e
printf '<<RAN:proc>>\n'
pgrep -fa 'rabbitmq\|activemq\|nats-server' 2>/dev/null | head -10
printf '<<RAN:ss>>\n'
ss -tln 2>/dev/null | grep -E ':(5672|15672|61616|61613|4222|8222) ' | head -8
printf '<<RAN:end>>\n'`
}

// mqProbes lists engine detection rules.
var mqEngineRules = []struct {
	procPattern string
	engine      string
	ports       []string
}{
	{"rabbitmq", "rabbitmq", []string{"5672", "15672"}},
	{"activemq", "activemq", []string{"61616", "61613"}},
	{"nats-server", "nats", []string{"4222", "8222"}},
}

func (p *RabbitmqActivemqNatsProbe) Parse(stdout string) (Result, error) {
	sections := splitTagSections(stdout, "RAN")
	proc := sections["proc"]
	ss := sections["ss"]

	activePorts := make(map[string]bool)
	for _, line := range strings.Split(ss, "\n") {
		if port := extractLastPort(line); port != "" {
			activePorts[port] = true
		}
	}

	var results []MQResult
	for _, rule := range mqEngineRules {
		hasProc := strings.Contains(proc, rule.procPattern)
		var foundPorts []int
		for _, p := range rule.ports {
			if activePorts[p] {
				foundPorts = appendIntUnique(foundPorts, portToInt(p))
			}
		}
		if !hasProc && len(foundPorts) == 0 {
			continue
		}
		res := MQResult{Engine: rule.engine, Ports: foundPorts}
		if hasProc && len(foundPorts) > 0 {
			res.Confidence = "high"
		} else if hasProc {
			res.Confidence = "medium"
		} else {
			res.Confidence = "low"
		}
		res.Evidence = []string{"process: " + rule.procPattern}
		results = append(results, res)
	}

	if len(results) == 0 {
		return Result{}, nil
	}
	return Result{MQ: results}, nil
}
