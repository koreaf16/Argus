// Package agent
// File: port_annotate.go
// Description: 네트워크 도구 결과에 well-known 포트 주석을 추가하는 레이어
// Responsibility: 포트 스캔 출력에 IANA 기반 포트-서비스 매핑 주석을 삽입하여 LLM의 추측 기반 식별을 방지

package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// wellKnownPorts는 IANA 표준 기반 포트-서비스 매핑이다.
// prompt_grounding.go의 테이블과 동기화를 유지한다.
var wellKnownPorts = map[int]string{
	22:    "SSH",
	80:    "HTTP",
	443:   "HTTPS",
	1521:  "Oracle DB",
	2181:  "ZooKeeper",
	2379:  "etcd (Kubernetes 클라이언트)",
	2380:  "etcd (Kubernetes 피어)",
	2888:  "ZooKeeper 피어",
	3000:  "Grafana",
	3306:  "MySQL / MariaDB",
	3888:  "ZooKeeper 리더 선출",
	4001:  "etcd (레거시)",
	5432:  "PostgreSQL",
	5672:  "RabbitMQ AMQP",
	6379:  "Redis",
	6443:  "kube-apiserver",
	7001:  "WebLogic HTTP",
	7002:  "WebLogic HTTPS",
	8080:  "HTTP Alt / Tomcat",
	8200:  "HashiCorp Vault",
	8443:  "HTTPS Alt",
	8500:  "Consul",
	9090:  "Prometheus",
	9092:  "Kafka",
	9099:  "Prometheus Alt",
	9200:  "Elasticsearch REST",
	9300:  "Elasticsearch Transport",
	10248: "kubelet Healthz",
	10250: "kubelet API",
	10256: "kube-proxy",
	10257: "kube-controller-manager",
	10259: "kube-scheduler",
	15672: "RabbitMQ 관리 UI",
	27017: "MongoDB",
}

// portPattern은 출력에서 포트 번호를 추출하는 정규식이다.
// ss, netstat, Get-NetTCPConnection 출력 형식을 모두 커버한다.
var portPattern = regexp.MustCompile(`[:\s](\d{2,5})\s`)

// annotatePortOutput는 network_info 또는 포트 스캔 결과처럼 보이는 shell_exec 출력에
// well-known 포트 주석을 추가한다. 해당되지 않는 출력은 변경 없이 반환한다.
func annotatePortOutput(toolName, raw string) string {
	if !shouldAnnotate(toolName, raw) {
		return raw
	}

	found := extractWellKnownFromOutput(raw)
	if len(found) == 0 {
		return raw
	}

	var sb strings.Builder
	sb.WriteString(raw)
	sb.WriteString("\n\n[Port annotations — IANA 기반, 실제 서비스는 process_list로 확인 필요]\n")
	for port, service := range found {
		sb.WriteString(fmt.Sprintf("  :%d → %s\n", port, service))
	}

	return sb.String()
}

// shouldAnnotate는 주석 레이어를 적용할지 판단한다.
func shouldAnnotate(toolName, raw string) bool {
	if toolName == "network_info" {
		return true
	}
	if toolName == "shell_exec" && strings.Contains(raw, "LISTEN") {
		return true
	}
	return false
}

// extractWellKnownFromOutput는 출력에서 well-known 포트를 찾아 서비스 이름과 함께 반환한다.
func extractWellKnownFromOutput(raw string) map[int]string {
	matches := portPattern.FindAllStringSubmatch(raw, -1)
	result := make(map[int]string)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		port, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if service, ok := wellKnownPorts[port]; ok {
			result[port] = service
		}
	}
	return result
}
