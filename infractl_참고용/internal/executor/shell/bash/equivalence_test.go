// Package bash
// File: equivalence_test.go
// Description: testdata/equivalence/*.yaml 동등성 테스트 로더
// Responsibility: 55개 케이스에서 Analyze 결과가 expect_deny / expect_readonly 와 일치하는지 검증

package bash

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/yourorg/infractl/internal/executor/shell"
)

// equivCase 는 equivalence YAML 의 단일 케이스를 나타낸다.
type equivCase struct {
	ID             string `yaml:"id"`
	Tool           string `yaml:"tool"`
	Command        string `yaml:"command"`
	ExpectDeny     *bool  `yaml:"expect_deny"`
	ExpectReadonly *bool  `yaml:"expect_readonly"`
}

type equivFile struct {
	Cases []equivCase `yaml:"cases"`
}

// isDenied 는 Analysis 의 위험도가 자동 차단 임계값 이상인지 반환한다.
// LevelHigh 이상이면 deny (UserApproval 은 승인 후 통과 = not deny).
func isDenied(a shell.Analysis) bool {
	return a.RiskLevel == shell.LevelHigh || a.RiskLevel == shell.LevelCritical
}

func TestEquivalence(t *testing.T) {
	pattern := filepath.Join("..", "..", "..", "..", "testdata", "equivalence", "*.yaml")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		t.Fatalf("equivalence YAML 파일을 찾을 수 없습니다 (pattern=%s): %v", pattern, err)
	}

	p := new(Provider)
	total, passed := 0, 0

	for _, fpath := range files {
		data, err := os.ReadFile(fpath)
		if err != nil {
			t.Fatalf("파일 읽기 실패 %s: %v", fpath, err)
		}
		var ef equivFile
		if err := yaml.Unmarshal(data, &ef); err != nil {
			t.Fatalf("YAML 파싱 실패 %s: %v", fpath, err)
		}

		for _, tc := range ef.Cases {
			total++
			if tc.Tool != "bash" && tc.Tool != "shell_exec" && tc.Tool != "exec" && tc.Tool != "shell" {
				// 비 bash 도구는 건너뜀 (pass-through)
				passed++
				continue
			}
			if tc.Command == "" {
				passed++
				continue
			}

			a, err := p.Analyze(context.Background(), tc.Command)
			if err != nil {
				t.Errorf("[%s] Analyze(%q) error: %v", tc.ID, tc.Command, err)
				continue
			}

			ok := true
			if tc.ExpectDeny != nil {
				got := isDenied(a)
				if got != *tc.ExpectDeny {
					t.Errorf("[%s] %q: deny=%v want %v (level=%v, findings=%v)",
						tc.ID, tc.Command, got, *tc.ExpectDeny, a.RiskLevel, a.Findings)
					ok = false
				}
			}
			if tc.ExpectReadonly != nil {
				if a.IsReadOnly != *tc.ExpectReadonly {
					t.Errorf("[%s] %q: readonly=%v want %v",
						tc.ID, tc.Command, a.IsReadOnly, *tc.ExpectReadonly)
					ok = false
				}
			}
			if ok {
				passed++
			}
		}
	}

	t.Logf("equivalence: %d/%d passed", passed, total)
}
