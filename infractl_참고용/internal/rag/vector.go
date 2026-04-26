// Package rag
// File: vector.go
// Description: 벡터 연산 유틸리티 — 코사인 유사도, 직렬화/역직렬화
// Responsibility: float32 벡터의 코사인 유사도 계산 및 BLOB 변환

package rag

import (
	"encoding/binary"
	"fmt"
	"math"
)

// CosineSimilarity는 두 벡터의 코사인 유사도를 반환한다 (0.0 ~ 1.0).
// 벡터 길이가 다르거나 0벡터이면 0을 반환한다.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		fa := float64(a[i])
		fb := float64(b[i])
		dot += fa * fb
		normA += fa * fa
		normB += fb * fb
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	sim := dot / denom
	// 부동소수점 오차로 1.0을 초과할 수 있으므로 클램핑
	if sim > 1.0 {
		sim = 1.0
	} else if sim < -1.0 {
		sim = -1.0
	}
	return float32(sim)
}

// SerializeVector는 []float32를 little-endian BLOB으로 직렬화한다.
func SerializeVector(v []float32) []byte {
	buf := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// DeserializeVector는 little-endian BLOB을 []float32로 역직렬화한다.
func DeserializeVector(b []byte) ([]float32, error) {
	if len(b)%4 != 0 {
		return nil, fmt.Errorf("invalid vector blob length: %d (must be multiple of 4)", len(b))
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		bits := binary.LittleEndian.Uint32(b[i*4:])
		v[i] = math.Float32frombits(bits)
	}
	return v, nil
}

// ScoredID는 유사도 점수와 지식 ID를 묶는다.
type ScoredID struct {
	ID    int64
	Score float32
}

// TopK는 후보 목록에서 minScore 이상인 상위 k개를 점수 내림차순으로 반환한다.
func TopK(candidates []ScoredID, k int, minScore float32) []ScoredID {
	// 조건 필터링
	var filtered []ScoredID
	for _, c := range candidates {
		if c.Score >= minScore {
			filtered = append(filtered, c)
		}
	}

	// 삽입 정렬 (k가 작고 데이터가 수천 건 이하이므로 충분)
	for i := 1; i < len(filtered); i++ {
		for j := i; j > 0 && filtered[j].Score > filtered[j-1].Score; j-- {
			filtered[j], filtered[j-1] = filtered[j-1], filtered[j]
		}
	}

	if k > 0 && len(filtered) > k {
		return filtered[:k]
	}
	return filtered
}
