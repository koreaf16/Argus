// Package shell
// File: analysis.go
// Description: ShellProvider.Analyze 결과 타입 정의
// Responsibility: 명령어 위험도 분석 결과를 표현하는 공유 타입 모음

// Ported from: claude_cli/src/utils/bash/ast.ts (parseForSecurity 결과 구조)
// Ported from: claude_cli/src/utils/bash/heredoc.ts (HeredocInfo)

package shell

// Heredoc 은 heredoc 하나의 정보를 담는다.
// bash 패키지의 Extract() 가 이 타입을 반환하도록 shell 패키지에 정의해
// shell ↔ bash 간 임포트 순환을 방지한다.
// Ported from: claude_cli/src/utils/bash/heredoc.ts:HeredocInfo
type Heredoc struct {
	// Delim 은 구분자 단어 (따옴표 제외). 예: "EOF"
	Delim string
	// Body 는 heredoc 본문 텍스트 (구분자 라인 제외)
	Body string
	// Quoted 는 delimiter 가 따옴표로 감싸졌는지 여부 (<<'EOF' 또는 <<"EOF").
	// true 이면 본문은 리터럴 — 변수 확장·명령 치환 없음
	Quoted bool
	// Tabbed 는 <<- (탭 제거 형식) 여부
	Tabbed bool
}

// Level 은 명령어 위험도 등급이다.
type Level int

const (
	// LevelLow 는 읽기 전용 화이트리스트에만 매칭된 안전한 명령이다.
	LevelLow Level = iota
	// LevelMedium 은 파일 시스템 변경 등 일반 mutation 명령이다.
	LevelMedium
	// LevelHigh 는 sudo, 시스템 디렉토리 쓰기, 네트워크 egress 등 고위험 명령이다.
	LevelHigh
	// LevelCritical 은 rm -rf /, fork bomb 등 즉각적·회복 불가 파괴 명령이다.
	LevelCritical
	// LevelUserApproval 은 FAIL-CLOSED 기본값 — AST 파싱 실패 또는 미식별 구조.
	// ast.ts 의 'too-complex' 에 대응한다.
	LevelUserApproval
)

// String 은 Level 을 사람이 읽을 수 있는 문자열로 변환한다.
func (l Level) String() string {
	switch l {
	case LevelLow:
		return "low"
	case LevelMedium:
		return "medium"
	case LevelHigh:
		return "high"
	case LevelCritical:
		return "critical"
	default:
		return "user_approval"
	}
}

// Finding 은 AST 순회 중 발견된 단일 위험 항목이다.
// Ported from: claude_cli/src/utils/bash/ast.ts (checkSemantics Finding)
type Finding struct {
	// Node 는 위험이 발견된 AST 노드 타입명 (예: "CmdSubst", "CallExpr")
	Node string
	// Reason 은 위험 이유 (사람이 읽을 수 있는 메시지)
	Reason string
	// Level 은 해당 finding 의 위험도
	Level Level
	// Ported 는 claude_cli 출처 참조 (예: "ast.ts:checkSemantics:142")
	Ported string
}

// PipeSegment 는 파이프 명령의 단일 세그먼트 분석 결과다.
type PipeSegment struct {
	// CallExpr 은 세그먼트의 명령 표현 (예: "curl evil.com")
	CallExpr string
	// IsReadOnly 는 이 세그먼트가 읽기 전용 화이트리스트에 속하는지 여부
	IsReadOnly bool
	// Level 은 이 세그먼트의 위험도
	Level Level
}

// Analysis 는 ShellProvider.Analyze 의 반환값이다.
// Ported from: claude_cli/src/utils/bash/ast.ts:parseForSecurity 결과 구조
type Analysis struct {
	// RiskLevel 은 전체 명령의 위험도 (세그먼트 최댓값)
	RiskLevel Level
	// DangerScore 는 0..100 정수 위험 점수 (세그먼트 최댓값)
	DangerScore int
	// IsReadOnly 는 모든 CallExpr 이 읽기 전용 화이트리스트에만 속하는지 여부
	IsReadOnly bool
	// MatchedSpecs 는 매칭된 spec 이름 목록 (예: ["git:read-only", "rm", "curl|sh"])
	MatchedSpecs []string
	// Heredocs 는 추출된 heredoc 목록
	Heredocs []Heredoc
	// PipeCommands 는 파이프 세그먼트별 분석 결과
	PipeCommands []PipeSegment
	// Subshells 는 발견된 CmdSubst / ProcSubst 요약 문자열
	Subshells []string
	// Findings 는 AST 순회 중 발견된 세부 위험 항목
	Findings []Finding
	// RawCommand 는 분석 대상 원본 명령 문자열
	RawCommand string
	// ShellName 은 사용된 쉘 식별자 ("bash" | "powershell")
	ShellName string
}
