package promptinput

import "strings"

type Theme struct {
	Name                    string
	TopBorderColor          string
	BottomBorderColor       string
	PromptIndicatorColor    string
	PromptTextColor         string
	FooterLabelColor        string
	FooterValueColor        string
	SuggestionColor         string
	SuggestionSelectedColor string
	SuggestionHelpColor     string
	InputBoxBg              string // background color for the input area; empty = no bg
	IndicatorChar           string // override indicator char, empty = "> "
	IndicatorColor          string // override indicator color when IndicatorChar is set
}

func ResolveTheme(name string) Theme {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "", "default", "argus_ui_demo", "argus-ui-demo", "ui_demo", "ui-demo", "gemini", "claude":
		return Theme{
			Name:                    "argus_ui_demo",
			TopBorderColor:          "#AFAFAF",
			BottomBorderColor:       "#AFAFAF",
			PromptIndicatorColor:    "#A06EE1",
			PromptTextColor:         "#FFFFFF",
			FooterLabelColor:        "#AFAFAF",
			FooterValueColor:        "#FFFFFF",
			SuggestionColor:         "#555555",
			SuggestionSelectedColor: "#4285F4",
			SuggestionHelpColor:     "#555555",
			InputBoxBg:              "#262626",
		}
	case "classic":
		return Theme{
			Name:                    "classic",
			TopBorderColor:          "240",
			BottomBorderColor:       "240",
			PromptIndicatorColor:    "14",
			PromptTextColor:         "250",
			FooterLabelColor:        "238",
			FooterValueColor:        "253",
			SuggestionColor:         "244",
			SuggestionSelectedColor: "14",
			SuggestionHelpColor:     "241",
			InputBoxBg:              "0",
		}
	default:
		return ResolveTheme("default")
	}
}
