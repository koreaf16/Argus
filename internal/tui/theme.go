package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/koreaf16/argus/internal/components/promptinput"
)

type uiTheme struct {
	Name string
	Variant string

	MutedColor   string
	BodyColor    string
	StalledColor string

	UserTitleColor       string
	AssistantTitleColor  string
	ThinkingTitleColor   string
	ToolUseTitleColor    string
	ToolResultTitleColor string
	NoticeTitleColor     string
	SystemTitleColor     string
	ErrorTitleColor      string
	PlanTitleColor       string
	ApprovalTitleColor   string
	ClaudeAccentColor    string

	ModalBorderColor string
	ModalTitleColor  string

	StatusLeftColor         string
	StatusLeftStalledColor  string
	StatusRightColor        string
	StatusRightStalledColor string
	ModeYOLOColor           string
	ModePlanColor           string
	ModeAutoColor           string

	MarkdownHeading1Color    string
	MarkdownHeading2Color    string
	MarkdownHeading3Color    string
	MarkdownHeading4Color    string
	MarkdownInlineCodeColor  string
	MarkdownInlineCodeBg     string
	MarkdownLinkColor        string
	MarkdownStrikeColor      string
	MarkdownQuoteColor       string
	MarkdownRuleColor        string
	MarkdownCodeHeadColor    string
	MarkdownCodeLineColor    string
	MarkdownTableBorderColor string
	MarkdownTableHeaderColor string
	MarkdownTableRowColor    string

	AssistantMarkerColor string
	UserMarkerColor      string
	UserBubbleBg         string
	UserBubbleFg         string
	InputBoxBg           string

	GlimmerTrailColors []string
	SpinnerFrames      []string

	PromptTheme promptinput.Theme
	DisableANSI bool

	AccentBarStyle lipgloss.Style
	ToolLabelStyle lipgloss.Style
	ChevronStyle   lipgloss.Style
}

func resolveUITheme(name string, aiDebug bool) uiTheme {
	return resolveUIThemeWithVariant(name, "current", aiDebug)
}

func resolveUIThemeWithVariant(name, variant string, aiDebug bool) uiTheme {
	normalized := strings.ToLower(strings.TrimSpace(name))
	variant = strings.ToLower(strings.TrimSpace(variant))
	if variant == "" {
		variant = "current"
	}
	switch normalized {
	case "", "default", "argus_ui_demo", "argus-ui-demo", "ui_demo", "ui-demo", "gemini", "claude":
		t := uiTheme{
			Name:                     "argus_ui_demo",
			Variant:                  variant,
			MutedColor:               "#999999",
			BodyColor:                "#FFFFFF",
			StalledColor:             "#FF6B80",
			UserTitleColor:           "#4EBA65",
			AssistantTitleColor:      "#4285F4",
			ThinkingTitleColor:       "#F49B4F", // Orange
			ToolUseTitleColor:        "#4285F4",
			ToolResultTitleColor:     "#4FB3BF",
			NoticeTitleColor:         "#4FB3BF",
			SystemTitleColor:         "#A06EE1",
			ErrorTitleColor:          "#FF6B80",
			PlanTitleColor:           "#A06EE1",
			ApprovalTitleColor:       "#B1B9F9",
			ClaudeAccentColor:        "#D77757",
			ModalBorderColor:         "#A06EE1",
			ModalTitleColor:          "#FFFFFF",
			StatusLeftColor:          "#FFFFFF",
			StatusLeftStalledColor:   "#EA4335",
			StatusRightColor:         "#AFAFAF",
			StatusRightStalledColor:  "#EA4335",
			ModeYOLOColor:            "#FF6B80",
			ModePlanColor:            "#48968C",
			ModeAutoColor:            "#4285F4",
			MarkdownHeading1Color:    "#4285F4",
			MarkdownHeading2Color:    "#4FB3BF",
			MarkdownHeading3Color:    "#A06EE1",
			MarkdownHeading4Color:    "#AFAFAF",
			MarkdownInlineCodeColor:  "#4FB3BF",
			MarkdownInlineCodeBg:     "#1A2030",
			MarkdownLinkColor:        "#4285F4",
			MarkdownStrikeColor:      "#555555",
			MarkdownQuoteColor:       "#AFAFAF",
			MarkdownRuleColor:        "#555555",
			MarkdownCodeHeadColor:    "#AFAFAF",
			MarkdownCodeLineColor:    "#AFAFAF",
			MarkdownTableBorderColor: "#888888",
			MarkdownTableHeaderColor: "#4285F4",
			MarkdownTableRowColor:    "#E0E0E0",
			AssistantMarkerColor:     "#A06EE1",
			UserMarkerColor:          "#555555",
			UserBubbleBg:             "",
			UserBubbleFg:             "#AFAFAF",
			InputBoxBg:               "#262626",
			GlimmerTrailColors:       []string{"#A06EE1", "#4285F4", "#4FB3BF", "#34A853", "#FBBC04", "#EA4335"},
			SpinnerFrames:            []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
			PromptTheme:              promptinput.ResolveTheme("argus_ui_demo"),
		}

		t.AccentBarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color(t.ThinkingTitleColor)).
			PaddingLeft(2).
			Margin(1, 0)

		t.ToolLabelStyle = lipgloss.NewStyle().
			Bold(true).
			MarginBottom(0)

		t.ChevronStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.ThinkingTitleColor)).
			Bold(true)

		return withAIDebug(applyVariant(t, variant), aiDebug)
	case "classic":
		t := uiTheme{
			Name:                     "classic",
			Variant:                  variant,
			MutedColor:               "243",
			BodyColor:                "250",
			StalledColor:             "160",
			UserTitleColor:           "40",
			AssistantTitleColor:      "14",
			ThinkingTitleColor:       "110",
			ToolUseTitleColor:        "11",
			ToolResultTitleColor:     "6",
			NoticeTitleColor:         "12",
			SystemTitleColor:         "13",
			ErrorTitleColor:          "9",
			PlanTitleColor:           "5",
			ApprovalTitleColor:       "11",
			ClaudeAccentColor:        "208",
			ModalBorderColor:         "11",
			ModalTitleColor:          "15",
			StatusLeftColor:          "250",
			StatusLeftStalledColor:   "160",
			StatusRightColor:         "243",
			ModeYOLOColor:            "9",
			ModePlanColor:            "40",
			ModeAutoColor:            "11",
			MarkdownHeading1Color:    "14",
			MarkdownHeading2Color:    "12",
			MarkdownHeading3Color:    "111",
			MarkdownHeading4Color:    "244",
			MarkdownInlineCodeColor:  "214",
			MarkdownLinkColor:        "14",
			MarkdownStrikeColor:      "240",
			MarkdownQuoteColor:       "244",
			MarkdownRuleColor:        "240",
			MarkdownCodeHeadColor:    "244",
			MarkdownCodeLineColor:    "249",
			MarkdownTableBorderColor: "245",
			MarkdownTableHeaderColor: "14",
			MarkdownTableRowColor:    "250",
			AssistantMarkerColor:     "214",
			UserMarkerColor:          "14",
			UserBubbleBg:             "236",
			UserBubbleFg:             "250",
			InputBoxBg:               "235",
			GlimmerTrailColors:       []string{"255", "250", "247", "244", "241", "239"},
			SpinnerFrames:            []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
			PromptTheme:              promptinput.ResolveTheme("classic"),
		}

		t.AccentBarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(lipgloss.Color(t.ThinkingTitleColor)).
			PaddingLeft(2).
			Margin(1, 0)

		t.ToolLabelStyle = lipgloss.NewStyle().
			Bold(true).
			MarginBottom(0)

		t.ChevronStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(t.ThinkingTitleColor)).
			Bold(true)

		return withAIDebug(applyVariant(t, variant), aiDebug)
	default:
		return resolveUIThemeWithVariant("default", variant, aiDebug)
	}
}

func applyVariant(t uiTheme, variant string) uiTheme {
	switch variant {
	case "argus-signature":
		t.BodyColor = "#F9FAFB"
		t.MutedColor = "#9AA0A6"
		t.StalledColor = "#D96570"
		t.ThinkingTitleColor = "#F49B4F"
		t.ToolUseTitleColor = "#5CC4D6"
		t.ToolResultTitleColor = "#61CF8E"
		t.NoticeTitleColor = "#4492E6"
		t.SystemTitleColor = "#F49B4F"
		t.ErrorTitleColor = "#D96570"
		t.PlanTitleColor = "#5CC4D6"
		t.ApprovalTitleColor = "#F5C451"
		t.ClaudeAccentColor = "#F49B4F"
		t.StatusRightColor = "#9AA0A6"
		t.ModeAutoColor = "#4492E6"
		t.ModePlanColor = "#61CF8E"
		t.ModeYOLOColor = "#D96570"
		t.AssistantMarkerColor = "#F49B4F"
		t.UserMarkerColor = "#5CC4D6"
		t.InputBoxBg = "#1D1F24"
		t.GlimmerTrailColors = []string{"#F49B4F", "#5CC4D6", "#4492E6", "#61CF8E", "#F5C451", "#D96570"}
		t.PromptTheme.PromptIndicatorColor = "#F49B4F"
		t.PromptTheme.PromptTextColor = "#F9FAFB"
		t.PromptTheme.FooterLabelColor = "#9AA0A6"
		t.PromptTheme.FooterValueColor = "#F9FAFB"
		t.PromptTheme.SuggestionSelectedColor = "#5CC4D6"
		t.PromptTheme.InputBoxBg = "#1D1F24"
	case "minimal-pro":
		t.BodyColor = "#F0F0F0"
		t.MutedColor = "#8A8A8A"
		t.StalledColor = "#C8C8C8"
		t.ThinkingTitleColor = "#D0D0D0"
		t.ToolUseTitleColor = "#C0C0C0"
		t.ToolResultTitleColor = "#B8B8B8"
		t.NoticeTitleColor = "#B0B0B0"
		t.SystemTitleColor = "#B8B8B8"
		t.ErrorTitleColor = "#E0E0E0"
		t.PlanTitleColor = "#C8C8C8"
		t.ApprovalTitleColor = "#D0D0D0"
		t.ClaudeAccentColor = "#D0D0D0"
		t.StatusLeftColor = "#F0F0F0"
		t.StatusRightColor = "#8A8A8A"
		t.ModeAutoColor = "#C0C0C0"
		t.ModePlanColor = "#D0D0D0"
		t.ModeYOLOColor = "#E0E0E0"
		t.AssistantMarkerColor = "#C8C8C8"
		t.UserMarkerColor = "#A8A8A8"
		t.InputBoxBg = "#1E1E1E"
		t.GlimmerTrailColors = []string{"#D0D0D0", "#C8C8C8", "#B8B8B8", "#A8A8A8"}
		t.PromptTheme.PromptIndicatorColor = "#D0D0D0"
		t.PromptTheme.PromptTextColor = "#F0F0F0"
		t.PromptTheme.FooterLabelColor = "#8A8A8A"
		t.PromptTheme.FooterValueColor = "#F0F0F0"
		t.PromptTheme.SuggestionSelectedColor = "#C8C8C8"
		t.PromptTheme.InputBoxBg = "#1E1E1E"
	default:
		// current: keep existing palette.
	}

	t.AccentBarStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color(t.ThinkingTitleColor)).
		PaddingLeft(2).
		Margin(1, 0)

	t.ToolLabelStyle = lipgloss.NewStyle().
		Bold(true).
		MarginBottom(0)

	t.ChevronStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(t.ThinkingTitleColor)).
		Bold(true)

	return t
}

func withAIDebug(t uiTheme, aiDebug bool) uiTheme {
	if !aiDebug {
		return t
	}
	t.DisableANSI = true
	t.SpinnerFrames = []string{"."}
	return t
}

func (t uiTheme) style(color string) lipgloss.Style {
	if t.DisableANSI {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color))
}
