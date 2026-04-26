// Package tui
// File: styles.go
// Description: TUI 컴포넌트에서 사용하는 lipgloss 스타일 상수 정의
// Responsibility: 색상, 테두리, 레이아웃 스타일의 중앙 관리

package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Claude 브랜드 색상 (Orange)
	ColorClaude = lipgloss.Color("#D77757")
	// 사용자 메시지 배경색
	ColorUserMsgBg = lipgloss.Color("#373737")
	// 성공/실패/경고
	ColorSuccess = lipgloss.Color("#4EBA65")
	ColorError   = lipgloss.Color("#FF6B80")
	ColorWarning = lipgloss.Color("#FFC107")
	// 보조 텍스트 및 구분선
	ColorSubtle = lipgloss.Color("#444444")
	ColorDim    = lipgloss.Color("#777777")

	// 입력창 border (더 연한 회색으로)
	ColorPromptBorder = lipgloss.Color("#444444")
	// Shimmer 애니메이션
	ColorShimmerBase = lipgloss.Color("#777777")
	ColorShimmerMid  = lipgloss.Color("#AAAAAA")
	ColorShimmerPeak = lipgloss.Color("#FFFFFF")
	// 응답 브래킷 / 진행 트리
	ColorBracket  = lipgloss.Color("#666666")
	ColorTreeLine = lipgloss.Color("#555555")

	// 하단 정보바 화살표
	StyleInfoBarArrow = lipgloss.NewStyle().
				Foreground(ColorClaude).
				Bold(true)

	// 하단 정보바 dim 텍스트
	StyleInfoBarDim = lipgloss.NewStyle().
			Foreground(ColorDim)

	// 사용자 입력 프롬프트
	StyleUserPrompt = lipgloss.NewStyle().
			Foreground(lipgloss.Color("6")).
			Bold(true)

	// Thinking 텍스트 (Claude Orange, 이탤릭체 제거하여 정갈하게)
	StyleThinking = lipgloss.NewStyle().
			Foreground(ColorClaude)

	// CmdBox 테두리 (Claude 스타일, 내부 패딩 소폭 확대)
	StyleCmdBoxBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorSubtle).
				PaddingLeft(2).
				PaddingRight(2).
				MarginTop(1).
				MarginBottom(1)

	// CmdBox 세이프가드 테두리 (노란색)
	StyleCmdBoxGuardBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorWarning).
				PaddingLeft(2).
				PaddingRight(2).
				MarginTop(1).
				MarginBottom(1)

	// CmdBox 취소/플랜모드 테두리 (인디고/보라색)
	StyleCmdBoxCancelBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPlanRequest).
				PaddingLeft(2).
				PaddingRight(2).
				MarginTop(1).
				MarginBottom(1)

	// CmdBox 헤더 (도구명)
	StyleCmdBoxToolName = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Bold(true)

	// CmdBox 헤더 (서버명)
	StyleCmdBoxTarget = lipgloss.NewStyle().
				Foreground(lipgloss.Color("6"))

	// CmdBox 헤더 (dim)
	StyleCmdBoxDim = lipgloss.NewStyle().
			Foreground(ColorDim)

	// 성공/실패 표시
	StyleSuccess = lipgloss.NewStyle().Foreground(ColorSuccess)
	StyleError   = lipgloss.NewStyle().Foreground(ColorError)
	StyleWarning = lipgloss.NewStyle().Foreground(ColorWarning)

	// 스피너
	StyleSpinner = lipgloss.NewStyle().Foreground(ColorClaude)

	// diff 색상 (Claude 스타일)
	StyleDiffAdd     = lipgloss.NewStyle().Foreground(ColorSuccess)
	StyleDiffDel     = lipgloss.NewStyle().Foreground(ColorError)
	StyleDiffHunk    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	StyleDiffHeader  = lipgloss.NewStyle().Foreground(ColorDim)
	StyleDiffLineNum = lipgloss.NewStyle().Foreground(ColorSubtle)

	// 붙여넣기 블록
	StylePasteBlock = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(ColorSubtle).
			Foreground(ColorDim).
			PaddingLeft(1).
			PaddingRight(1)

	// 입력바 프롬프트
	StyleInputPrompt = lipgloss.NewStyle().
				Foreground(ColorClaude).
				Bold(true)

	// 구분선
	StyleSeparator = lipgloss.NewStyle().Foreground(ColorSubtle)

	// 배너 타이틀
	StyleBannerTitle = lipgloss.NewStyle().
				Foreground(ColorClaude).
				Bold(true)

	// 배너 정보 (dim)
	StyleBannerInfo = lipgloss.NewStyle().
			Foreground(ColorDim)

	// 배너 강조
	StyleBannerHighlight = lipgloss.NewStyle().
				Foreground(ColorWarning)

	// 채팅 메시지 스타일
	StyleClaudeMsg = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	StyleUserMsg = lipgloss.NewStyle().
			Background(ColorUserMsgBg).
			Foreground(lipgloss.Color("255")).
			PaddingLeft(1).
			PaddingRight(1)

	// 입력 border 렌더링
	StylePromptBorder = lipgloss.NewStyle().
				Foreground(ColorPromptBorder)

	// Assistant 응답 "  ⎿  " 브래킷
	StyleResponseBracket = lipgloss.NewStyle().
				Foreground(ColorBracket)

	// Footer dim 구분자 "·"
	StyleFooterDim = lipgloss.NewStyle().
			Foreground(ColorDim)

	// Footer 키보드 힌트
	StyleFooterHint = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#BBBBBB"))

	// Diff 라인번호 gutter
	StyleDiffGutter = lipgloss.NewStyle().
			Foreground(ColorSubtle)

	// 쉘 실행 중 "Running..." 표시
	StyleShellRunning = lipgloss.NewStyle().
				Foreground(ColorClaude).
				Italic(true)

	// 진행 트리 라인 (├─ └─ │)
	StyleTreeLine = lipgloss.NewStyle().
			Foreground(ColorTreeLine)

	// 입력 큐 표시
	ColorQueueIndicator = lipgloss.Color("#FFC107")
	StyleQueueCount     = lipgloss.NewStyle().
				Foreground(ColorQueueIndicator).
				Bold(true)
	StyleQueuePreview = lipgloss.NewStyle().
				Foreground(ColorDim)

	// 인터랙티브 셀렉션
	StyleSelectionQuestion = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CCCCCC")).
				Bold(true)
	StyleSelectionCursor = lipgloss.NewStyle().
				Foreground(ColorClaude).
				Bold(true)
	StyleSelectionOption = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#AAAAAA"))
	StyleSelectionSelected = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF"))

	// 리치 셀렉션 — 태그 배지
	StyleSelectionTag = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#000000")).
				Background(ColorClaude).
				PaddingLeft(1).
				PaddingRight(1).
				Bold(true)

	// 리치 셀렉션 — 서브 설명 (Recommended 등)
	StyleSelectionSub = lipgloss.NewStyle().
				Foreground(ColorDim).
				Italic(true)

	// 리치 셀렉션 — 미리보기 패널 테두리
	StylePreviewBox = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorSubtle).
				Foreground(lipgloss.Color("#CCCCCC")).
				PaddingLeft(1).
				PaddingRight(1)

	// 리치 셀렉션 — 키보드 힌트
	StyleSelectionHint = lipgloss.NewStyle().
				Foreground(ColorDim)

	// ── Gemini CLI 스타일 박스형 선택 UI ───────────────────────────────
	// 박스 테두리 색상 (프롬프트 보더와 일치)
	ColorGeminiBox = lipgloss.Color("#4EBA65") // ColorSuccess 와 동일한 초록색 계열로 변경하여 선택 UI 통일

	// 박스 헤더 라벨 (볼드, 테마색)
	StyleGeminiHeader = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Bold(true)

	// ● 불릿 — 현재 선택된 항목 표시
	StyleGeminiBullet = lipgloss.NewStyle().
				Foreground(ColorSuccess).
				Bold(true)

	// 선택된 항목 번호+라벨 (음영 스타일 - 초록 바탕에 검정 글씨)
	StyleGeminiSelected = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#000000")).
				Background(ColorSuccess).
				Bold(true).
				PaddingLeft(1).
				PaddingRight(1)

	// 비선택 항목 번호+라벨
	StyleGeminiOption = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#AAAAAA"))

	// 옵션 서브 설명 (들여쓰기)
	StyleGeminiSubDesc = lipgloss.NewStyle().
				Foreground(ColorDim)

	// 하단 키보드 힌트
	StyleGeminiHint = lipgloss.NewStyle().
				Foreground(ColorDim)

	// Claude CLI "permission" 색상 — 인라인 코드 강조 (파란-보라)
	ColorPermission = lipgloss.Color("#B1B9F9")

	// 보조 스타일 (회색)
	StyleSubtle = lipgloss.NewStyle().Foreground(ColorSubtle)

	// 전체 박스 컨테이너
	StyleGeminiBox = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(ColorGeminiBox).
				PaddingLeft(1).
				PaddingRight(1)

	// Phase 11: Task proposal UI colors
	ColorPlanRequest  = lipgloss.Color("#6366F1") // indigo — task proposal waiting
	ColorPlanApproved = ColorSuccess               // reuse
	ColorPlanRejected = ColorError                 // reuse
	ColorSubagent     = lipgloss.Color("#22D3EE") // cyan — future subagent

	// Task proposal card styles
	StyleTaskProposalOuter = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPlanRequest).
				PaddingLeft(1).
				PaddingRight(1).
				MarginTop(1).
				MarginBottom(1)

	StyleTaskProposalInner = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorSubtle).
				PaddingLeft(1).
				PaddingRight(1)

	StyleTaskProposalOK = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPlanApproved).
				PaddingLeft(1).
				PaddingRight(1).
				MarginTop(1).
				MarginBottom(1)

	StyleTaskProposalDeny = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPlanRejected).
				PaddingLeft(1).
				PaddingRight(1).
				MarginTop(1).
				MarginBottom(1)

	StyleTaskBadgeRoot    = lipgloss.NewStyle().Background(ColorError).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 1)
	StyleTaskBadgeAccount = lipgloss.NewStyle().Background(ColorWarning).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 1)
	StyleTaskTitle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))
	StyleTaskDim          = lipgloss.NewStyle().Foreground(ColorDim)
)
