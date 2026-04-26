// Package tui
// File: service_table.go
// Description: /services 슬래시 명령용 서버→서비스→서브인스턴스 3계층 뷰 렌더링
// Responsibility: discovered_services, connectors, active states를 계층 문자열로 포맷팅

package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/yourorg/infractl/internal/connector"
	"github.com/yourorg/infractl/internal/store"
)

// serviceStatus는 서비스의 현재 상태 수준이다.
type serviceStatus int

const (
	statusDiscovered serviceStatus = iota // discover_services로 탐지됨
	statusSaved                           // connectors 테이블에 저장됨
	statusActive                          // 현재 세션에서 활성 상태
)

func serviceStatusLabel(s serviceStatus) string {
	switch s {
	case statusActive:
		return "[활성 ✓]"
	case statusSaved:
		return "[저장됨]"
	default:
		return "[탐지됨]"
	}
}

// serviceSummary는 계층 뷰에서 하나의 서비스를 요약한다.
type serviceSummary struct {
	ServiceType string
	ServiceName string
	Port        int
	Status      serviceStatus
	ToolCount   int
	SubItems    []subItemSummary
}

// subItemSummary는 서비스 하위 인스턴스(Oracle PDB, K8s namespace 등)를 나타낸다.
type subItemSummary struct {
	Name   string
	Status serviceStatus
}

// buildServicesView는 서버→서비스→서브인스턴스 계층 문자열을 반환한다.
func (m AppModel) buildServicesView(ctx context.Context, serverFilter string) string {
	servers, err := m.store.List(ctx)
	if err != nil {
		return fmt.Sprintf("서버 목록 조회 실패: %s", err)
	}

	if serverFilter != "" {
		var filtered []store.Server
		for _, s := range servers {
			if strings.EqualFold(s.Name, serverFilter) {
				filtered = append(filtered, s)
			}
		}
		servers = filtered
	}

	if len(servers) == 0 {
		if serverFilter != "" {
			return fmt.Sprintf("서버 '%s'를 찾을 수 없습니다.", serverFilter)
		}
		return "등록된 서버가 없습니다. server_add 도구로 서버를 등록하세요."
	}

	// 탐지된 서비스 수집
	discoveredMap := make(map[string][]store.DiscoveredServiceEntry)
	if m.discoveryStore != nil {
		for _, s := range servers {
			entries, _ := m.discoveryStore.ListDiscoveredServices(ctx, s.Name)
			discoveredMap[s.Name] = entries
		}
	}

	// 저장된 커넥터 수집
	var savedConnectors []store.ConnectorEntry
	if m.connectorMgr != nil {
		savedConnectors, _ = m.connectorMgr.ListSavedConnectors(ctx)
	}

	// 활성 커넥터 수집
	var activeStates []connector.ConnectorState
	if m.connectorMgr != nil {
		activeStates = m.connectorMgr.States()
	}

	return buildServiceHierarchy(servers, m.activeServer, discoveredMap, savedConnectors, activeStates)
}

// buildServiceHierarchy는 계층 텍스트를 생성한다.
func buildServiceHierarchy(
	servers []store.Server,
	activeServer *store.Server,
	discoveredMap map[string][]store.DiscoveredServiceEntry,
	savedConnectors []store.ConnectorEntry,
	activeStates []connector.ConnectorState,
) string {
	// 활성 상태를 key→state 맵으로 변환
	activeByKey := make(map[string]connector.ConnectorState)
	for _, s := range activeStates {
		activeByKey[s.Name] = s
	}

	// 저장된 커넥터를 "server/type/serviceName" → entry 맵으로
	savedByLookup := make(map[string]store.ConnectorEntry)
	for _, e := range savedConnectors {
		k := e.ServerName + "/" + e.ServiceType + "/" + e.ServiceName
		savedByLookup[k] = e
	}

	var sb strings.Builder
	sb.WriteString("서버-서비스 목록:\n")

	for _, srv := range servers {
		isActive := activeServer != nil && srv.Name == activeServer.Name
		marker := "  "
		if isActive {
			marker = "● "
		}
		sb.WriteString(fmt.Sprintf("\n%s%s  (%s@%s:%d)",
			marker, srv.Name, srv.User, srv.Host, srv.Port))
		if srv.OS != "" {
			sb.WriteString("  " + srv.OS)
		}
		sb.WriteString("\n")

		summaries := collectServiceSummaries(srv.Name, discoveredMap[srv.Name], savedByLookup, activeByKey)

		if len(summaries) == 0 {
			sb.WriteString("    (서비스 없음 — discover_services로 탐색하세요)\n")
			continue
		}

		for _, svc := range summaries {
			label := serviceStatusLabel(svc.Status)
			toolInfo := ""
			if svc.Status == statusActive && svc.ToolCount > 0 {
				toolInfo = fmt.Sprintf("  %d개 도구", svc.ToolCount)
			}
			portStr := ""
			if svc.Port > 0 {
				portStr = fmt.Sprintf(":%d", svc.Port)
			}
			sb.WriteString(fmt.Sprintf("    %-10s %-16s %-6s  %s%s\n",
				svc.ServiceType, svc.ServiceName, portStr, label, toolInfo))

			if len(svc.SubItems) > 0 {
				parts := make([]string, 0, len(svc.SubItems))
				for _, sub := range svc.SubItems {
					parts = append(parts, fmt.Sprintf("%s %s", sub.Name, serviceStatusLabel(sub.Status)))
				}
				sb.WriteString(fmt.Sprintf("               └ %s\n", strings.Join(parts, ", ")))
			}
		}
	}

	sb.WriteString("\n사용: /services <서버명>  — 특정 서버만 조회")
	return sb.String()
}

// collectServiceSummaries는 한 서버의 서비스 목록을 탐지·저장·활성 데이터에서 통합 집계한다.
func collectServiceSummaries(
	serverName string,
	discovered []store.DiscoveredServiceEntry,
	savedByLookup map[string]store.ConnectorEntry,
	activeByKey map[string]connector.ConnectorState,
) []serviceSummary {
	type summaryKey struct{ svcType, name string }
	byKey := make(map[summaryKey]*serviceSummary)

	// 1. 탐지된 서비스 등록
	for _, d := range discovered {
		k := summaryKey{d.ServiceType, d.ServiceName}
		if _, exists := byKey[k]; !exists {
			byKey[k] = &serviceSummary{
				ServiceType: d.ServiceType,
				ServiceName: d.ServiceName,
				Port:        d.Port,
				Status:      statusDiscovered,
			}
		}
		svc := byKey[k]
		// PDB 목록 파싱 (details["pdbs"])
		if pdbList := d.Details["pdbs"]; pdbList != "" {
			for _, pdb := range strings.Split(pdbList, ",") {
				pdb = strings.TrimSpace(pdb)
				if pdb == "" {
					continue
				}
				pdbStatus := statusDiscovered
				// active key 형식: "server/type/cdb/pdb"
				activeKey := serverName + "/" + d.ServiceType + "/" + d.ServiceName + "/" + pdb
				// saved lookup key 형식: "server/type/cdb/pdb" (serviceName이 "cdb/pdb"로 저장)
				subSavedKey := serverName + "/" + d.ServiceType + "/" + d.ServiceName + "/" + pdb
				if _, ok := activeByKey[activeKey]; ok {
					pdbStatus = statusActive
				} else if _, ok := savedByLookup[subSavedKey]; ok {
					pdbStatus = statusSaved
				}
				svc.SubItems = appendOrUpdateSubItem(svc.SubItems, pdb, pdbStatus)
			}
		}
		// K8s namespace 목록 파싱 (details["namespaces"])
		if nsList := d.Details["namespaces"]; nsList != "" {
			for _, ns := range strings.Split(nsList, ",") {
				ns = strings.TrimSpace(ns)
				if ns == "" {
					continue
				}
				svc.SubItems = appendOrUpdateSubItem(svc.SubItems, ns, statusDiscovered)
			}
		}
	}

	// 2. 저장된 커넥터로 상태 업그레이드
	for _, e := range savedByLookup {
		if e.ServerName != serverName {
			continue
		}
		name, sub := splitStoredServiceName(e.ServiceName)
		k := summaryKey{e.ServiceType, name}
		if existing, exists := byKey[k]; exists {
			if sub == "" && existing.Status < statusSaved {
				existing.Status = statusSaved
			}
			if sub != "" {
				existing.SubItems = appendOrUpdateSubItem(existing.SubItems, sub, statusSaved)
			}
		} else {
			svc := &serviceSummary{ServiceType: e.ServiceType, ServiceName: name, Status: statusSaved}
			if sub != "" {
				svc.SubItems = []subItemSummary{{Name: sub, Status: statusSaved}}
			}
			byKey[k] = svc
		}
	}

	// 3. 활성 커넥터로 상태 업그레이드
	for _, state := range activeByKey {
		if state.ServerName != serverName {
			continue
		}
		k := summaryKey{state.Type, state.ServiceName}
		if existing, exists := byKey[k]; exists {
			if state.SubInstance == "" {
				if existing.Status < statusActive {
					existing.Status = statusActive
				}
				existing.ToolCount = len(state.Tools)
			} else {
				existing.SubItems = appendOrUpdateSubItem(existing.SubItems, state.SubInstance, statusActive)
			}
		} else {
			svc := &serviceSummary{
				ServiceType: state.Type,
				ServiceName: state.ServiceName,
				Status:      statusActive,
				ToolCount:   len(state.Tools),
			}
			if state.SubInstance != "" {
				svc.SubItems = []subItemSummary{{Name: state.SubInstance, Status: statusActive}}
				svc.Status = statusDiscovered // 부모는 상태 미확정
			}
			byKey[k] = svc
		}
	}

	result := make([]serviceSummary, 0, len(byKey))
	for _, svc := range byKey {
		result = append(result, *svc)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ServiceType != result[j].ServiceType {
			return result[i].ServiceType < result[j].ServiceType
		}
		return result[i].ServiceName < result[j].ServiceName
	})
	return result
}

// appendOrUpdateSubItem은 SubItems에서 같은 이름 항목을 찾아 상태를 업그레이드하거나 새로 추가한다.
func appendOrUpdateSubItem(items []subItemSummary, name string, status serviceStatus) []subItemSummary {
	for i, item := range items {
		if strings.EqualFold(item.Name, name) {
			if items[i].Status < status {
				items[i].Status = status
			}
			return items
		}
	}
	return append(items, subItemSummary{Name: name, Status: status})
}

// splitStoredServiceName은 "name/sub" 또는 "name" 형식을 분리한다.
func splitStoredServiceName(s string) (name, sub string) {
	if idx := strings.Index(s, "/"); idx >= 0 {
		return s[:idx], s[idx+1:]
	}
	return s, ""
}
