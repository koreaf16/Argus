// Package constants — 경로 계산 헬퍼.
//
// 파일 역할: ~/.argus/ 설정 디렉토리와 그 하위 파일의 절대 경로를 플랫폼 중립적으로 계산.
//
//	os.UserHomeDir 이 실패하면 현재 작업 디렉토리 기준으로 폴백해 테스트 가능성을 유지.
//
// 포함 모듈:
//   - ConfigDir(): ~/.argus 절대 경로.
//   - SettingsPath(): ~/.argus/settings.json.
//   - ModelsPath(): ~/.argus/models.json.
//   - HistoryPath(): ~/.argus/history.
//
// 호출/사용 방식:
//   - cmd/argus/init.go 가 디렉토리·파일을 생성할 때.
//   - internal/services/llm/registry.go 가 모델 카탈로그를 로드/저장할 때.
//   - internal/repl 가 readline 히스토리 경로를 지정할 때.
//
// 연결:
//   - 표준 라이브러리 os, path/filepath 만 사용.
//   - 같은 패키지의 product.go 상수를 사용.
package constants

import (
	"os"
	"path/filepath"
)

// ConfigDir 는 사용자의 Argus 설정 디렉토리 절대 경로를 반환.
// 항상 현재 작업 디렉토리 기준(.Argus)으로 반환한다.
func ConfigDir() string {
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, ConfigDirName)
}

// SettingsPath 는 ~/.argus/settings.json 절대 경로.
func SettingsPath() string {
	return filepath.Join(ConfigDir(), SettingsFileName)
}

// ModelsPath 는 ~/.argus/models.json 절대 경로.
func ModelsPath() string {
	return filepath.Join(ConfigDir(), ModelsFileName)
}

// HistoryPath 는 ~/.argus/history 절대 경로 (REPL 히스토리).
func HistoryPath() string {
	return filepath.Join(ConfigDir(), HistoryFileName)
}

// CredentialsPath 는 .Argus/ssh_credentials.json 절대 경로.
func CredentialsPath() string {
	return filepath.Join(ConfigDir(), CredentialsFileName)
}
