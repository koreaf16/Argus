// Package connector
// File: discovery_validation.go
// Description: connector activation/probe preflight validation against discovery results
// Responsibility: enforce server/target consistency and recent discovery matching

package connector

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yourorg/infractl/internal/store"
)

const discoveryTTL = 15 * time.Minute

func validateDiscoveryTarget(execTarget, server string) error {
	if sameServerTarget(execTarget, server) {
		return nil
	}
	targetLabel := strings.TrimSpace(execTarget)
	if targetLabel == "" {
		targetLabel = "localhost"
	}
	return fmt.Errorf(
		"server 불일치: server=%q 이지만 현재 실행 워크스페이스는 %q 입니다. server 파라미터에 IP 대신 워크스페이스 alias를 사용하세요 (예: server=%q)",
		server, targetLabel, targetLabel,
	)
}

func validateRecentDiscovery(
	ctx context.Context,
	ds store.DiscoveryStore,
	server string,
	serviceType string,
	serviceName string,
) (store.DiscoveredServiceEntry, error) {
	if ds == nil {
		return store.DiscoveredServiceEntry{}, fmt.Errorf(
			"discovery 저장소가 연결되지 않았습니다. server=%s 대상으로 discover_services를 먼저 실행하세요",
			server,
		)
	}

	entries, err := ds.ListDiscoveredServices(ctx, server)
	if err != nil {
		return store.DiscoveredServiceEntry{}, fmt.Errorf("discover_services 결과 조회 실패: %w", err)
	}
	if len(entries) == 0 {
		return store.DiscoveredServiceEntry{}, fmt.Errorf(
			"최근 discovery 결과가 없습니다. server=%s 대상으로 discover_services를 먼저 실행하세요",
			server,
		)
	}

	latest := latestDiscoveredAt(entries)
	if latest.IsZero() || time.Since(latest) > discoveryTTL {
		return store.DiscoveredServiceEntry{}, fmt.Errorf(
			"최근 discovery 결과가 15분을 초과해 만료되었습니다. server=%s 대상으로 discover_services를 다시 실행하세요",
			server,
		)
	}

	for _, e := range entries {
		if strings.EqualFold(strings.TrimSpace(e.ServiceType), strings.TrimSpace(serviceType)) &&
			strings.EqualFold(strings.TrimSpace(e.ServiceName), strings.TrimSpace(serviceName)) {
			return e, nil
		}
	}

	return store.DiscoveredServiceEntry{}, fmt.Errorf(
		"최근 discovery 결과(server=%s)에 %s/%s 서비스가 없습니다. 해당 서버에서 discover_services를 다시 실행하세요",
		server, serviceType, serviceName,
	)
}

func latestDiscoveredAt(entries []store.DiscoveredServiceEntry) time.Time {
	var latest time.Time
	for _, e := range entries {
		if e.DiscoveredAt.After(latest) {
			latest = e.DiscoveredAt
		}
	}
	return latest
}

func buildServiceInfoFromDiscovery(
	server string,
	serviceType string,
	serviceName string,
	requestedPort int,
	entry store.DiscoveredServiceEntry,
) ServiceInfo {
	resolvedName := strings.TrimSpace(serviceName)
	if strings.TrimSpace(entry.ServiceName) != "" {
		resolvedName = entry.ServiceName
	}

	port := requestedPort
	if port == 0 {
		port = entry.Port
	}

	return ServiceInfo{
		ServerName:  server,
		ServiceType: serviceType,
		Name:        resolvedName,
		Port:        port,
		Details:     cloneDetails(entry.Details),
	}
}

func cloneDetails(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func sameServerTarget(a, b string) bool {
	return normalizeTarget(a) == normalizeTarget(b)
}

func normalizeTarget(v string) string {
	n := strings.ToLower(strings.TrimSpace(v))
	if n == "" || n == "localhost" || n == "local" {
		return "localhost"
	}
	return n
}
