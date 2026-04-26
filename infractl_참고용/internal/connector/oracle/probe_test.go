// Package oracle
// File: probe_test.go
// Description: parseProbeOutput 파싱 로직 단위 테스트
// Responsibility: sqlplus 출력 파싱 결과를 다양한 케이스에 대해 검증

package oracle

import (
	"testing"
)

func TestParseProbeOutput_CDBWithPDBs(t *testing.T) {
	output := `
VER:19.0.0.0.0
CDB:YES
PDB:HRPDB:READ WRITE
PDB:SALESPDB:MOUNTED
`
	got := parseProbeOutput(output)

	if got["oracle_version"] != "19.0.0.0.0" {
		t.Errorf("oracle_version: got %q, want %q", got["oracle_version"], "19.0.0.0.0")
	}
	if got["cdb"] != "yes" {
		t.Errorf("cdb: got %q, want %q", got["cdb"], "yes")
	}
	if got["pdbs"] != "HRPDB:READ WRITE,SALESPDB:MOUNTED" {
		t.Errorf("pdbs: got %q, want %q", got["pdbs"], "HRPDB:READ WRITE,SALESPDB:MOUNTED")
	}
}

func TestParseProbeOutput_NonCDB(t *testing.T) {
	output := `
VER:11.2.0.4.0
CDB:NO
`
	got := parseProbeOutput(output)

	if got["oracle_version"] != "11.2.0.4.0" {
		t.Errorf("oracle_version: got %q, want %q", got["oracle_version"], "11.2.0.4.0")
	}
	if got["cdb"] != "no" {
		t.Errorf("cdb: got %q, want %q", got["cdb"], "no")
	}
	if _, ok := got["pdbs"]; ok {
		t.Errorf("pdbs should not be set for non-CDB, got %q", got["pdbs"])
	}
}

func TestParseProbeOutput_VersionOnly_ORA00904(t *testing.T) {
	// Oracle 11g: SELECT cdb FROM v$database가 ORA-00904 반환
	output := `
VER:11.2.0.4.0
ORA-00904: "CDB": invalid identifier
`
	got := parseProbeOutput(output)

	if got["oracle_version"] != "11.2.0.4.0" {
		t.Errorf("oracle_version: got %q, want %q", got["oracle_version"], "11.2.0.4.0")
	}
	if got["cdb"] != "no" {
		t.Errorf("cdb should be 'no' when ORA-00904, got %q", got["cdb"])
	}
}

func TestParseProbeOutput_Empty(t *testing.T) {
	got := parseProbeOutput("")
	if len(got) != 0 {
		t.Errorf("empty output should yield empty map, got %v", got)
	}
}

func TestParseProbeOutput_NoPDBs_CDB(t *testing.T) {
	// CDB이지만 PDB가 없는 케이스 (PDB$SEED만 있고 이미 필터됨)
	output := `
VER:19.0.0.0.0
CDB:YES
`
	got := parseProbeOutput(output)

	if got["cdb"] != "yes" {
		t.Errorf("cdb: got %q, want %q", got["cdb"], "yes")
	}
	if _, ok := got["pdbs"]; ok {
		t.Errorf("pdbs should not be set when no PDBs, got %q", got["pdbs"])
	}
}
