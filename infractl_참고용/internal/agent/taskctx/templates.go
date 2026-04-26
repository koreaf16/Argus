// Package taskctx
// File: templates.go
// Description: Kind 별 KindFields 필수 키 검증 및 LLM 가이드 설명
// Responsibility: ValidateDraft, RequiredFieldsFor, KindFieldDescriptions

package taskctx

import (
	"fmt"
	"strings"
)

// KindFieldDescriptions 는 각 필드의 설명 (LLM 가이드용)이다.
var KindFieldDescriptions = map[string]string{
	// install
	"software_name":    "설치할 소프트웨어 이름 (예: Oracle Database)",
	"software_version": "설치할 버전 (예: 19.3.0)",
	"install_method":   "설치 방식: package|source|binary|container|official_installer 중 하나",
	"install_path":     "설치 절대경로 (예: /u01/app/oracle/product/19.3.0/dbhome_1)",
	"install_options":  "설치 옵션 (예: silent install with response file)",
	"owner_account":    "소유 계정 (예: oracle — root 금지, 서비스 계정 권장)",
	"post_install_tasks": "설치 후 작업 (예: DB 생성, 리스너 설정)",
	// service
	"service_name":      "서비스 이름 (예: sshd, oracle)",
	"action":            "수행 동작: start|stop|restart|reload|status",
	"expected_downtime": "예상 다운타임 (예: 약 30초)",
	"dependents":        "영향받는 다른 서비스 (예: WAS, nginx)",
	// configure
	"config_target":  "대상 파일 또는 명령 (예: /etc/ssh/sshd_config)",
	"change_summary": "변경 내용 요약 (예: Port=2222로 변경)",
	"reload_method":  "설정 반영 방법 (예: systemctl reload sshd)",
	// migrate
	"source":      "마이그레이션 원본 (예: orcl1)",
	"destination": "마이그레이션 대상 (예: orcl2)",
	"data_volume": "데이터 볼륨 (예: 약 120GB)",
	"dry_run_first": "드라이런 선행 여부: true|false",
	// backup
	"backup_target":   "백업 대상 (예: DB full)",
	"backup_location": "백업 경로 (예: /backup/20260417)",
	"retention":       "보존 기간 (예: 7d)",
	"verify_method":   "검증 방법 (예: restore 테스트)",
	// patch
	"patch_id":        "패치 ID (예: PSU 19.20)",
	"applies_to":      "패치 적용 대상 (예: DB home)",
	"prereqs_verified": "선결조건 확인 여부 (예: 디스크 10GB 여유)",
	"test_plan":       "적용 후 테스트 계획 (예: opatch lsinventory)",
}

// requiredFieldsByKind 는 Kind 별 필수 KindFields 키 목록이다.
var requiredFieldsByKind = map[TaskKind][]string{
	KindInstall: {
		"software_name",
		"software_version",
		"install_method",
		"install_path",
		"install_options",
		"owner_account",
		"post_install_tasks",
	},
	KindService: {
		"service_name",
		"action",
		"expected_downtime",
		"dependents",
	},
	KindConfigure: {
		"config_target",
		"change_summary",
		"reload_method",
	},
	KindMigrate: {
		"source",
		"destination",
		"data_volume",
		"dry_run_first",
	},
	KindBackup: {
		"backup_target",
		"backup_location",
		"retention",
		"verify_method",
	},
	KindPatch: {
		"patch_id",
		"applies_to",
		"prereqs_verified",
		"test_plan",
	},
	// inspect, other: 추가 필수 없음
	KindInspect: {},
	KindOther:   {},
}

// RequiredFieldsFor 는 LLM 시스템 프롬프트에 표시할 Kind 별 요구 필드 요약을 반환한다.
func RequiredFieldsFor(kind TaskKind) []string {
	fields, ok := requiredFieldsByKind[kind]
	if !ok {
		return []string{}
	}
	result := make([]string, len(fields))
	copy(result, fields)
	return result
}

// ValidateDraft 는 Draft 의 Kind 에 맞는 필수 필드 모두 채워졌는지 확인한다.
// 누락된 필드 리스트를 에러에 포함한다.
func ValidateDraft(d TaskContextDraft) error {
	required, ok := requiredFieldsByKind[d.Kind]
	if !ok {
		// 알 수 없는 Kind — 검증 통과 (관대하게)
		return nil
	}

	if len(required) == 0 {
		return nil
	}

	var missing []string
	for _, field := range required {
		val, exists := d.KindFields[field]
		if !exists || strings.TrimSpace(val) == "" {
			missing = append(missing, field)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("draft validation: missing required fields for kind %q: %v", d.Kind, missing)
	}

	return nil
}
