// Package context
// File: builder.go
// Description: 시스템 프롬프트 구성 요소를 병렬로 수집하는 빌더
// Responsibility: errgroup 기반 병렬 fetch — FetchSystemPromptParts 진입점

// Ported from: claude_cli/src/utils/api.ts (fetchSystemPromptParts pattern)

package context

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

// Parts 는 빌더가 수집한 프롬프트 조각 집합이다.
type Parts struct {
	UserMD  string
	EnvInfo EnvInfo
}

// FetchParts 는 프롬프트 구성 요소를 병렬로 수집한다.
func FetchParts(ctx context.Context) (Parts, error) {
	var (
		parts Parts
		mu    = make(chan struct{}, 1)
	)
	_ = mu

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		parts.UserMD = LoadUserContext()
		return nil
	})

	g.Go(func() error {
		parts.EnvInfo = GetEnvInfo(gctx)
		return nil
	})

	if err := g.Wait(); err != nil {
		return Parts{}, fmt.Errorf("context fetch: %w", err)
	}
	return parts, nil
}
