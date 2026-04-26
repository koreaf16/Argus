// Package cache
// File: stats.go
// Description: 캐시 통계 구조체 정의
// Responsibility: 캐시 히트/미스/eviction 통계 데이터 보관

package cache

// Stats는 Cache의 운영 통계이다.
type Stats struct {
	Hits             int64 // 캐시 히트 횟수
	Misses           int64 // 캐시 미스 횟수
	Evictions        int64 // LRU eviction 횟수
	ExpiredEvictions int64 // TTL 만료로 인한 eviction 횟수
}
