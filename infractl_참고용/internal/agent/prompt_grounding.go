// Package agent
// File: prompt_grounding.go
// Description: LLM 환각 방지를 위한 Grounding 규칙 시스템 프롬프트 섹션
// Responsibility: "추측 금지", "검증 불가 시 인정", 결과 정직 보고 등 환각 방지 지시 및 well-known 포트 참조 테이블 정의

package agent

import "strings"

// appendGroundingRules는 환각 방지를 위한 grounding 규칙 섹션을 시스템 프롬프트에 추가한다.
// Claude CLI의 "Never guess URLs", "If you can't verify say so", "Report outcomes faithfully" 패턴을
// 인프라 에이전트 컨텍스트에 맞게 적용한다.
func appendGroundingRules(sb *strings.Builder) {
	sb.WriteString("## Grounding Rules (CRITICAL — 위반 불가)\n")
	sb.WriteString("**아래 규칙은 다른 모든 지시보다 우선한다.**\n\n")

	sb.WriteString("1. **포트/서비스 미확인 시 추측 금지.**\n")
	sb.WriteString("   포트가 `discover_services` 결과나 아래 Well-known 목록에 없으면,\n")
	sb.WriteString("   '포트 XXXX — 서비스 미확인'으로 보고하고 `discover_services` 또는 `process_list`로 확인을 제안한다.\n\n")

	sb.WriteString("2. **검증 불가 사실은 명시적으로 인정한다.**\n")
	sb.WriteString("   '현재 데이터로 알 수 없다', '추가 확인 필요'라고 명확히 말한다.\n")
	sb.WriteString("   절대 추측을 사실처럼 제시하지 않는다.\n\n")

	sb.WriteString("3. **결과를 정직하게 보고한다.**\n")
	sb.WriteString("   에러가 발생했으면 에러를 보고한다. 실패를 성공으로 요약하거나,\n")
	sb.WriteString("   에러를 숨기거나, 실제와 다른 결과를 제시하지 않는다.\n\n")

	sb.WriteString("4. **전략을 바꾸기 전에 실패 원인을 진단한다.**\n")
	sb.WriteString("   명령이 실패하면 왜 실패했는지 분석을 먼저 제시하고 대안을 시도한다.\n")
	sb.WriteString("   같은 명령을 이유 없이 반복하지 않는다.\n\n")

	sb.WriteString("5. **증거 부재 ≠ 부재 증거.**\n")
	sb.WriteString("   '이 명령으로 찾지 못했다'는 '존재하지 않는다'가 아니다.\n")
	sb.WriteString("   무엇을 확인했고 무엇이 아직 미확인인지 명시한다.\n\n")

	sb.WriteString("6. **교정 후 재추측 금지.**\n")
	sb.WriteString("   사용자가 틀렸다고 지적하면, 다른 추측을 내놓지 말고\n")
	sb.WriteString("   실제 데이터를 확인하는 도구(`process_list`, `discover_services`)를 실행한다.\n\n")

	appendWellKnownPorts(sb)
}

// appendWellKnownPorts는 IANA 표준 기반 well-known 포트 참조 테이블을 추가한다.
// 포트 스캔 결과 해석 시 이 테이블을 먼저 참조하도록 안내한다.
func appendWellKnownPorts(sb *strings.Builder) {
	sb.WriteString("### Well-known 포트 참조 (포트 스캔 해석 시 활용)\n")
	sb.WriteString("아래 목록은 IANA 표준 기반이다. **실제 서비스는 다를 수 있으니 `process_list`로 반드시 확인한다.**\n\n")
	sb.WriteString("| 포트 | 서비스 | 비고 |\n")
	sb.WriteString("|------|--------|------|\n")
	sb.WriteString("| 22 | SSH | |\n")
	sb.WriteString("| 80 / 443 | HTTP / HTTPS | |\n")
	sb.WriteString("| 1521 | Oracle DB | 기본 리스너 |\n")
	sb.WriteString("| 2181 / 2888 / 3888 | ZooKeeper | |\n")
	sb.WriteString("| 2379 | etcd | Kubernetes 클라이언트 포트 |\n")
	sb.WriteString("| 2380 | etcd | Kubernetes 피어 포트 |\n")
	sb.WriteString("| 3306 | MySQL / MariaDB | |\n")
	sb.WriteString("| 4001 | etcd (레거시) | |\n")
	sb.WriteString("| 5432 | PostgreSQL | |\n")
	sb.WriteString("| 5672 / 15672 | RabbitMQ | 5672: AMQP, 15672: 관리 UI |\n")
	sb.WriteString("| 6379 | Redis | |\n")
	sb.WriteString("| 6443 | kube-apiserver | Kubernetes API |\n")
	sb.WriteString("| 7001 / 7002 | WebLogic | |\n")
	sb.WriteString("| 8080 / 8443 | Tomcat / HTTP Alt | |\n")
	sb.WriteString("| 8200 | HashiCorp Vault | |\n")
	sb.WriteString("| 8500 | Consul | |\n")
	sb.WriteString("| 9090 / 9099 | Prometheus | |\n")
	sb.WriteString("| 9092 | Kafka | |\n")
	sb.WriteString("| 9200 / 9300 | Elasticsearch | 9200: REST, 9300: transport |\n")
	sb.WriteString("| 10248 | kubelet | Healthz |\n")
	sb.WriteString("| 10250 | kubelet | API |\n")
	sb.WriteString("| 10256 | kube-proxy | |\n")
	sb.WriteString("| 10257 | kube-controller-manager | |\n")
	sb.WriteString("| 10259 | kube-scheduler | |\n")
	sb.WriteString("| 27017 | MongoDB | |\n")
	sb.WriteString("\n")
}
