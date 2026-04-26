// Package taskctx
// File: templates_test.go
// Description: ValidateDraft 및 RequiredFieldsFor 단위 테스트
// Responsibility: Kind 별 필수 필드 검증 및 요구 필드 목록 반환 확인

package taskctx

import (
	"strings"
	"testing"
)

func installKindFields() map[string]string {
	return map[string]string{
		"software_name":      "Oracle Database",
		"software_version":   "19.3.0",
		"install_method":     "binary",
		"install_path":       "/u01/app/oracle/product/19.3.0/dbhome_1",
		"install_options":    "silent install with response file",
		"owner_account":      "oracle",
		"post_install_tasks": "DB 생성, 리스너 설정",
	}
}

func TestValidateDraft_InstallAllFields_Nil(t *testing.T) {
	d := TaskContextDraft{
		Kind:       KindInstall,
		KindFields: installKindFields(),
	}
	if err := ValidateDraft(d); err != nil {
		t.Errorf("want nil error, got %v", err)
	}
}

func TestValidateDraft_InstallMissingSoftwareName_Error(t *testing.T) {
	fields := installKindFields()
	delete(fields, "software_name")

	d := TaskContextDraft{
		Kind:       KindInstall,
		KindFields: fields,
	}
	err := ValidateDraft(d)
	if err == nil {
		t.Fatal("want error for missing software_name, got nil")
	}
	if !strings.Contains(err.Error(), "software_name") {
		t.Errorf("error should mention 'software_name': %v", err)
	}
}

func TestValidateDraft_InstallMultipleMissing_AllInError(t *testing.T) {
	d := TaskContextDraft{
		Kind:       KindInstall,
		KindFields: map[string]string{},
	}
	err := ValidateDraft(d)
	if err == nil {
		t.Fatal("want error for empty KindFields, got nil")
	}
	// 모든 필수 필드가 에러에 언급되어야 함
	required := requiredFieldsByKind[KindInstall]
	for _, field := range required {
		if !strings.Contains(err.Error(), field) {
			t.Errorf("error should mention %q: %v", field, err)
		}
	}
}

func TestValidateDraft_InspectEmptyKindFields_Nil(t *testing.T) {
	d := TaskContextDraft{
		Kind:       KindInspect,
		KindFields: map[string]string{},
	}
	if err := ValidateDraft(d); err != nil {
		t.Errorf("inspect kind should need no extra fields, got %v", err)
	}
}

func TestValidateDraft_OtherNilKindFields_Nil(t *testing.T) {
	d := TaskContextDraft{
		Kind:       KindOther,
		KindFields: nil,
	}
	if err := ValidateDraft(d); err != nil {
		t.Errorf("other kind should need no extra fields, got %v", err)
	}
}

func TestRequiredFieldsFor_Install(t *testing.T) {
	fields := RequiredFieldsFor(KindInstall)
	if len(fields) == 0 {
		t.Fatal("install should have required fields")
	}
	// 핵심 필드 포함 확인
	required := map[string]bool{
		"software_name":    false,
		"install_method":   false,
		"install_path":     false,
		"owner_account":    false,
	}
	for _, f := range fields {
		if _, ok := required[f]; ok {
			required[f] = true
		}
	}
	for k, found := range required {
		if !found {
			t.Errorf("RequiredFieldsFor(install) missing %q", k)
		}
	}
}

func TestRequiredFieldsFor_Inspect_Empty(t *testing.T) {
	fields := RequiredFieldsFor(KindInspect)
	if len(fields) != 0 {
		t.Errorf("inspect should have no required fields, got %v", fields)
	}
}

func TestValidateDraft_Service_MissingAction(t *testing.T) {
	d := TaskContextDraft{
		Kind: KindService,
		KindFields: map[string]string{
			"service_name":      "nginx",
			"expected_downtime": "5초",
			"dependents":        "WAS",
			// "action" 누락
		},
	}
	err := ValidateDraft(d)
	if err == nil {
		t.Fatal("want error for missing action field")
	}
	if !strings.Contains(err.Error(), "action") {
		t.Errorf("error should mention 'action': %v", err)
	}
}
