// Package bash — 셸 세션 스냅샷 (환경변수 캡처).
//
// 파일 역할: 셸 세션 시작 시 환경변수를 캡처해 파일로 저장한다.
//
//	Phase 1 에서는 스텁 구현을 제공하며, Phase 2 에서 완전 구현한다.
//
// 포함 모듈:
//   - CreateAndSaveSnapshot: 셸 경로를 받아 스냅샷을 생성하고 저장 (Phase 1: 스텁)
//
// 호출/사용 방식:
//   - internal/bootstrap 패키지가 세션 초기화 시 호출
//
// 연결:
//   - 원본: src/utils/bash/ShellSnapshot.ts
package bash

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CreateAndSaveSnapshot 은 shellPath 셸을 이용해 환경변수 스냅샷을 생성·저장한다.
func CreateAndSaveSnapshot(shellPath string) (string, error) {
	// 셸을 실행하여 현재 환경변수와 alias 를 캡처한다.
	// bash -l -c "alias && export" 형식을 사용한다.
	cmd := exec.Command(shellPath, "-l", "-c", "alias && export")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to capture shell env: %w", err)
	}

	// 임시 파일 생성
	tmpDir := os.TempDir()
	snapshotFile := filepath.Join(tmpDir, fmt.Sprintf("argus-snapshot-%d.sh", os.Getpid()))

	// 캡처된 내용을 파일로 저장
	// alias 와 export 문을 그대로 저장하면 나중에 source 로 불러올 수 있다.
	if err := os.WriteFile(snapshotFile, output, 0644); err != nil {
		return "", fmt.Errorf("failed to save snapshot file: %w", err)
	}

	return snapshotFile, nil
}
