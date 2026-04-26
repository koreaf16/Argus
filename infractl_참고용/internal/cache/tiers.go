// Package cache
// File: tiers.go
// Description: 용도별 캐시 프리셋 팩토리
// Responsibility: 웹/쿼리/메타데이터 캐시를 권장 설정으로 생성하는 팩토리 함수 제공

package cache

import "time"

const (
	// 웹 캐시: 50MB, 15분 TTL
	webCacheMaxBytes = 50 * 1024 * 1024
	webCacheTTL      = 15 * time.Minute

	// 쿼리 결과 캐시: 10MB, 5분 TTL
	queryCacheMaxBytes = 10 * 1024 * 1024
	queryCacheTTL      = 5 * time.Minute

	// 메타데이터 캐시: 2MB, 30분 TTL
	metaCacheMaxBytes = 2 * 1024 * 1024
	metaCacheTTL      = 30 * time.Minute
)

// StringSizeFn은 string 값의 바이트 크기를 반환하는 sizeFn이다.
func StringSizeFn(s string) int64 {
	return int64(len(s))
}

// NewWebCache는 URL→Markdown 웹 콘텐츠 캐시를 생성한다.
// 최대 50MB, TTL 15분.
func NewWebCache() *Cache[string, string] {
	return NewCache[string, string](Config{
		MaxBytes: webCacheMaxBytes,
		TTL:      webCacheTTL,
		Name:     "web",
	}, StringSizeFn)
}

// NewQueryCache는 DB 쿼리 결과 캐시를 생성한다.
// 최대 10MB, TTL 5분.
func NewQueryCache() *Cache[string, string] {
	return NewCache[string, string](Config{
		MaxBytes: queryCacheMaxBytes,
		TTL:      queryCacheTTL,
		Name:     "query",
	}, StringSizeFn)
}

// NewMetadataCache는 서비스 메타데이터(서버 정보, 스키마 등) 캐시를 생성한다.
// 최대 2MB, TTL 30분.
func NewMetadataCache() *Cache[string, string] {
	return NewCache[string, string](Config{
		MaxBytes: metaCacheMaxBytes,
		TTL:      metaCacheTTL,
		Name:     "metadata",
	}, StringSizeFn)
}
