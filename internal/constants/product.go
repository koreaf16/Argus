// Package constants — 프로젝트 전역 상수 모음.
//
// 파일 역할: 제품 메타데이터 (이름·버전·저장 경로 관례) 를 한곳에서 관리한다.
//
//	변경 시 모든 모듈이 공통으로 본다.
//
// 포함 모듈:
//   - Name / DisplayName: 바이너리·표시 이름.
//   - Version: 빌드 시 -ldflags 로 덮어쓸 수 있도록 var 로 둠.
//   - ConfigDirName / HistoryFileName / SettingsFileName / ModelsFileName: ~/.argus/ 하위 파일 관례.
//
// 호출/사용 방식:
//   - cmd/argus/help.go, cmd/argus/version.go, cmd/argus/init.go 가 제품명·경로를 읽을 때.
//   - internal/services/llm/registry.go 가 models.json 파일명을 읽을 때.
//
// 연결:
//   - import 없음 (이 패키지는 다른 internal/* 에 의존하지 않음 — 순환 방지용 최하위 계층).
//   - 많은 상위 패키지가 이 파일을 import 한다.
package constants

// Name 은 바이너리 이름 (argus.exe → "argus").
const Name = "argus"

// DisplayName 은 UI·도움말에 표시되는 사람이 읽는 이름.
const DisplayName = "Argus"

// Version 은 빌드 시 -ldflags "-X ..." 로 주입한다. 기본값은 dev.
var Version = "0.0.1-dev"

// Commit 은 빌드 시 -ldflags 로 주입한다.
var Commit = "unknown"

// ConfigDirName 은 사용자 홈 아래 Argus 설정 디렉토리 이름. 실제 경로는
// constants.ConfigDir() 로 얻는다 (paths.go 참조).
const ConfigDirName = ".Argus"

// SettingsFileName 은 ConfigDirName 내부의 사용자 설정 파일명.
const SettingsFileName = "settings.json"

// ModelsFileName 은 ConfigDirName 내부의 모델 카탈로그 파일명.
const ModelsFileName = "models.json"

// HistoryFileName 은 REPL readline 히스토리 파일명.
const HistoryFileName = "history"

// CredentialsFileName 은 ConfigDirName 내부의 암호화된 자격증명 파일명.
const CredentialsFileName = "ssh_credentials.json"
