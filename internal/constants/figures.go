// Package constants — 터미널 출력용 유니코드 기호.
//
// 파일 역할: UI 렌더링에 사용하는 특수문자·기호 상수를 한 곳에 모은다.
//
//	lipgloss 컴포넌트, 스피너, 상태 표시줄 등에서 참조한다.
//
// 포함 모듈:
//   - BlackCircle / BulletOperator / TeardropAsterisk 등 주요 도형 기호
//   - UpArrow / DownArrow / LightningBolt 방향·상태 기호
//   - EffortLow~EffortMax 작업 부하 표시 기호
//   - BridgeSpinnerFrames / BridgeReadyIndicator 등 브리지 상태 기호
//
// 호출/사용 방식:
//   - internal/repl/render.go, internal/components 등 렌더링 레이어에서 직접 참조.
//
// 연결:
//   - 원본: src/constants/figures.ts
package constants

// BlackCircle — Claude CLI 스타일의 ● 기호.
var BlackCircle = "●"

const BulletOperator = "∙"
const TeardropAsterisk = "✻"
const UpArrow = "\u2191"   // ↑
const DownArrow = "\u2193" // ↓
const LightningBolt = "↯"  // \u21af

// 작업 부하 표시 기호
const EffortLow = "○"    // \u25cb
const EffortMedium = "◐" // \u25d0
const EffortHigh = "●"   // \u25cf
const EffortMax = "◉"    // \u25c9

// 미디어/트리거 상태 표시
const PlayIcon = "\u25b6"  // ▶
const PauseIcon = "\u23f8" // ⏸

// MCP 구독 표시
const RefreshArrow = "\u21bb"  // ↻
const ChannelArrow = "\u2190"  // ←
const InjectedArrow = "\u2192" // →
const ForkGlyph = "\u2442"     // ⑂

// 리뷰 상태 표시 (다이아몬드 상태)
const DiamondOpen = "\u25c7"   // ◇ 실행 중
const DiamondFilled = "\u25c6" // ◆ 완료/실패
const ReferenceMark = "\u203b" // ※

// 이슈 플래그 표시
const FlagIcon = "\u2691" // ⚑

// 인용 구분 기호
const BlockquoteBar = "\u258e"   // ▎
const HeavyHorizontal = "\u2501" // ━

// 브리지 스피너 프레임
var BridgeSpinnerFrames = []string{
	"\u00b7|\u00b7",
	"\u00b7/\u00b7",
	"\u00b7\u2014\u00b7",
	"\u00b7\\\u00b7",
}

const BridgeReadyIndicator = "\u00b7\u2714\ufe0e\u00b7"
const BridgeFailedIndicator = "\u00d7"
