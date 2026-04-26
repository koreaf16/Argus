// Package checkpoint
// File: types.go
// Description: 체크포인트 패키지 진입점 - store.Checkpoint 타입 재사용
// Responsibility: checkpoint 패키지가 store 패키지의 Checkpoint 타입을 래핑 없이 사용

// 타입 정의는 internal/store/checkpoint_store.go에 있다.
// checkpoint 패키지는 store.Checkpoint, store.CheckpointSnapshot을 직접 사용한다.
package checkpoint
