// Package connector
// File: types.go
// Description: 커넥터 시스템의 핵심 인터페이스 및 공유 타입 정의
// Responsibility: Connector 인터페이스, 상태 타입, ServiceInfo, Credentials, SaveMode 정의

package connector

import (
	"context"

	"github.com/yourorg/infractl/internal/executor"
	"github.com/yourorg/infractl/internal/tools"
)

// ConnectorStatus는 커넥터 연결 상태를 나타낸다.
type ConnectorStatus string

const (
	StatusConnected    ConnectorStatus = "connected"
	StatusConnecting   ConnectorStatus = "connecting"
	StatusError        ConnectorStatus = "error"
	StatusDisconnected ConnectorStatus = "disconnected"
)

// SaveMode는 커넥터 접속 정보의 저장 방식을 결정한다.
type SaveMode string

const (
	SavePermanent SaveMode = "permanent" // SQLite에 영구 저장
	SaveSession   SaveMode = "session"   // 메모리에만 보관, 종료 시 삭제
	SaveNone      SaveMode = "none"      // 저장 안 함 — 매번 질문
)

// ServiceInfo는 발견된 서비스 인스턴스를 식별하는 정보이다.
type ServiceInfo struct {
	ServerName  string
	ServiceType string            // "oracle", "mysql" 등
	Name        string            // Oracle CDB SID, MySQL DB명 등
	SubInstance string            // Oracle PDB명, K8s namespace 등 (선택적 하위 단위)
	Port        int
	Details     map[string]string // 서비스별 추가 정보
}

// Credentials는 서비스 접속에 필요한 복호화된 자격증명이다.
type Credentials struct {
	Username string
	Password string
	Role     string // Oracle sysdba 등 역할
	OSAuth   bool   // true = OS 인증 (패스워드 없이 접속)
}

// OSAuthProber는 패스워드 없이 OS 수준 인증을 시도할 수 있는 커넥터의 선택적 인터페이스이다.
// Oracle "/ as sysdba", MySQL unix socket, PostgreSQL peer 인증 등에 사용한다.
type OSAuthProber interface {
	// ProbeOSAuth는 SSH executor를 통해 OS 인증을 시도한다.
	// 성공 시 OSAuth=true인 Credentials를 반환하고, 실패 시 non-nil error를 반환한다.
	ProbeOSAuth(ctx context.Context, info ServiceInfo, exec executor.Executor) (Credentials, error)
}

// MetadataProber는 활성화 직전에 서비스 메타데이터를 탐색하여 ServiceInfo.Details에
// 추가 정보를 보강하는 옵셔널 인터페이스이다 (Oracle CDB/버전, MySQL 버전 등).
// 실패해도 활성화를 차단하지 않으므로 error는 경고 로그용이고 부분 결과만 반환해도 된다.
type MetadataProber interface {
	ProbeMetadata(ctx context.Context, info ServiceInfo, creds Credentials, exec executor.Executor) (map[string]string, error)
}

// ConnectorState는 커넥터 인스턴스의 런타임 상태이다.
type ConnectorState struct {
	Name        string            // 예: "server/oracle/CDB/PDB" 또는 "server/oracle/ORCL"
	ServerName  string
	Type        string
	ServiceName string
	SubInstance string            // PDB명, namespace 등 하위 단위 (없으면 빈 문자열)
	Status      ConnectorStatus
	Error       string
	Tools       []string          // 활성화된 도구 이름 목록
	Details     map[string]string // 서비스 메타데이터 (oracle_version, cdb, pdbs 등)
}

// Connector는 서비스별 커넥터가 구현해야 하는 인터페이스이다.
// 접속 성공 시 전용 도구 세트를 생성하고, 상태를 관리한다.
type Connector interface {
	// ServiceType은 이 커넥터가 담당하는 서비스 타입을 반환한다.
	ServiceType() string

	// GenerateTools는 접속 성공 시 LLM이 사용할 도구 세트를 생성한다.
	// 생성된 도구들은 커넥터 상태와 연동된 IsEnabled()를 구현해야 한다.
	GenerateTools(info ServiceInfo, creds Credentials) []tools.Tool

	// ToolNames는 이 커넥터가 생성한 도구 이름 목록을 반환한다.
	// Registry.Unregister() 호출 시 사용한다.
	ToolNames() []string

	// Status는 현재 연결 상태를 반환한다.
	Status() ConnectorStatus
}

// connectorKey는 커넥터를 고유하게 식별하는 복합 키를 생성한다.
// subInstance가 비어있지 않으면 키에 포함한다 (예: "server/oracle/CDB/PDB").
func connectorKey(serverName, serviceType, name, subInstance string) string {
	key := serverName + "/" + serviceType + "/" + name
	if subInstance != "" {
		key += "/" + subInstance
	}
	return key
}

// ConfirmationHandler는 외부 커넥터 실행 전 사용자에게 승인을 요청하는 인터페이스이다.
type ConfirmationHandler interface {
	Confirm(ctx context.Context, title, message string) (bool, error)
}
