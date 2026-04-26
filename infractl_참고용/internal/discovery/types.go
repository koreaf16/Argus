// Package discovery
// File: types.go
// Description: 서비스 탐색 관련 타입 정의
// Responsibility: 탐색 결과, 서비스 패턴, 확신도 기반 상태를 표현하는 공유 타입

package discovery

import "time"

// ServiceType은 탐지 가능한 인프라 서비스 종류를 나타낸다.
type ServiceType string

const (
	ServiceOracle     ServiceType = "oracle"
	ServiceMySQL      ServiceType = "mysql"
	ServicePostgreSQL ServiceType = "postgresql"
	ServiceTomcat     ServiceType = "tomcat"
	ServiceWebLogic   ServiceType = "weblogic"
	ServiceEtcd       ServiceType = "etcd"
	ServiceKubeAPI    ServiceType = "kube-apiserver"
	ServiceKubelet    ServiceType = "kubelet"
	ServicePrometheus ServiceType = "prometheus"
	ServiceSystemd    ServiceType = "systemd"
	ServiceDocker     ServiceType = "docker"
	ServiceNginx      ServiceType = "nginx"
	ServiceApache     ServiceType = "apache"
	ServiceHAProxy    ServiceType = "haproxy"
	ServiceTraefik    ServiceType = "traefik"
	ServiceSSL        ServiceType = "ssl"
	ServiceVHost      ServiceType = "vhost"
	ServiceUnknown    ServiceType = "unknown"
)

// ConfidenceLevel은 탐지 확신도 단계를 나타낸다.
type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"   // 0.7 이상 — 자동 보고
	ConfidenceMedium ConfidenceLevel = "medium" // 0.5~0.7 — 사용자에게 확인 질문
	ConfidenceLow    ConfidenceLevel = "low"    // 0.5 미만 — 보고만
)

// 확신도 임계값 상수
const (
	ThresholdHigh   = 0.7
	ThresholdMedium = 0.5
)

// ServicePattern은 서비스 타입별 매칭 기준을 정의한다.
type ServicePattern struct {
	Type         ServiceType
	ProcessRegex string   // ps args 컬럼에 대한 정규식
	DefaultPorts []int    // 체크할 기본 포트 목록
	ConfigFiles  []string // 존재 여부를 확인할 설정 파일 경로
}

// DiscoveredService는 탐색 중 발견된 단일 서비스 인스턴스이다.
type DiscoveredService struct {
	ServerName string
	Type       ServiceType
	Name       string            // 예: Oracle SID, MySQL 데이터베이스명
	PID        int
	Port       int
	User       string
	Confidence float64
	Level      ConfidenceLevel
	Details    map[string]string // 임의 추가 정보 (home_dir, version 등)
}

// ScanResult는 특정 서버 대상 탐색의 전체 결과를 담는다.
type ScanResult struct {
	ServerName string
	Services   []DiscoveredService
	ScannedAt  time.Time
}

// CalcLevel은 확신도 값에 따라 ConfidenceLevel을 반환한다.
func CalcLevel(confidence float64) ConfidenceLevel {
	switch {
	case confidence >= ThresholdHigh:
		return ConfidenceHigh
	case confidence >= ThresholdMedium:
		return ConfidenceMedium
	default:
		return ConfidenceLow
	}
}
