package probes

import (
	"context"
	"strings"
	"time"
)

// KafkaZookeeperProbe detects Apache Kafka and Zookeeper brokers.
type KafkaZookeeperProbe struct{}

func (p *KafkaZookeeperProbe) Name() string { return "kafka_zookeeper" }

func (p *KafkaZookeeperProbe) PreferredTimeout() time.Duration { return 3 * time.Second }

func (p *KafkaZookeeperProbe) Run(ctx context.Context, fn ProbeExec) (Result, error) {
	out, err := fn(ctx, p.ScriptFragment())
	if err != nil {
		return Result{}, err
	}
	return p.Parse(out)
}

func (p *KafkaZookeeperProbe) ScriptFragment() string {
	return `set +e
printf '<<KZ:proc>>\n'
pgrep -fa 'kafka.Kafka\|QuorumPeerMain\|kafka.server' 2>/dev/null | head -10
printf '<<KZ:ss>>\n'
ss -tln 2>/dev/null | grep -E ':(9092|9093|2181|2182|2888|3888) ' | head -8
printf '<<KZ:end>>\n'`
}

func (p *KafkaZookeeperProbe) Parse(stdout string) (Result, error) {
	sections := splitTagSections(stdout, "KZ")
	proc := sections["proc"]
	ss := sections["ss"]

	hasKafkaProc := strings.Contains(proc, "kafka.Kafka") || strings.Contains(proc, "kafka.server")
	hasZKProc := strings.Contains(proc, "QuorumPeerMain")

	kafkaPorts := []int{}
	zkPorts := []int{}
	for _, line := range strings.Split(ss, "\n") {
		port := extractLastPort(line)
		switch port {
		case "9092", "9093":
			kafkaPorts = appendIntUnique(kafkaPorts, portToInt(port))
		case "2181", "2182", "2888", "3888":
			zkPorts = appendIntUnique(zkPorts, portToInt(port))
		}
	}

	var results []MQResult

	if hasKafkaProc || len(kafkaPorts) > 0 {
		res := MQResult{Engine: "kafka", Ports: kafkaPorts}
		if hasKafkaProc && len(kafkaPorts) > 0 {
			res.Confidence = "high"
		} else if hasKafkaProc {
			res.Confidence = "medium"
		} else {
			res.Confidence = "low"
		}
		res.Evidence = []string{"process: kafka.Kafka"}
		results = append(results, res)
	}

	if hasZKProc || len(zkPorts) > 0 {
		res := MQResult{Engine: "zookeeper", Ports: zkPorts}
		if hasZKProc && len(zkPorts) > 0 {
			res.Confidence = "high"
		} else if hasZKProc {
			res.Confidence = "medium"
		} else {
			res.Confidence = "low"
		}
		res.Evidence = []string{"process: QuorumPeerMain"}
		results = append(results, res)
	}

	if len(results) == 0 {
		return Result{}, nil
	}
	return Result{MQ: results}, nil
}
