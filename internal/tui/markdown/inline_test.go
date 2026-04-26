package markdown

import (
	"testing"
)

func TestStripInlineMarkers(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "30%", "30%"},
		{"bold", "**70G**", "70G"},
		{"bold_italic", "***text***", "text"},
		{"italic_star", "*italic*", "italic"},
		{"italic_under", "_italic_", "italic"},
		{"strikethrough", "~~strike~~", "strike"},
		{"inline_code", "`crawl4ai`", "crawl4ai"},
		{"inline_code_path", "`/home`", "/home"},
		{"link", "[doc](https://example.com)", "doc (https://example.com)"},
		{"underline", "<u>text</u>", "text"},
		{"url_kept", "https://example.com", "https://example.com"},
		{"mixed", "**70G** used", "70G used"},
		{"korean_bold", "**서비스**", "서비스"},
		{"no_markers", "21G", "21G"},
		{"empty", "", ""},
		{"backtick_port", "`11235:30090`", "11235:30090"},
		{"bold_path", "**/ (Root)**", "/ (Root)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := StripInlineMarkers(tc.input)
			if got != tc.want {
				t.Errorf("StripInlineMarkers(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseInlineWithBase_DisableANSI(t *testing.T) {
	p := Palette{DisableANSI: true}
	cases := []struct {
		input string
		want  string
	}{
		{"**70G**", "70G"},
		{"`crawl4ai`", "crawl4ai"},
		{"plain text", "plain text"},
		{"", ""},
	}
	for _, tc := range cases {
		got := ParseInlineWithBase(tc.input, "#FFFFFF", false, p)
		if got != tc.want {
			t.Errorf("ParseInlineWithBase(%q, DisableANSI) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseInlineWithBase_NoPanic(t *testing.T) {
	p := Palette{
		Body:        "#FFFFFF",
		TableRow:    "#E0E0E0",
		TableHeader: "#4285F4",
		InlineCode:  "#4FB3BF",
		Link:        "#4285F4",
		Strike:      "#555555",
	}
	inputs := []string{
		"**bold**",
		"`code`",
		"*italic*",
		"[text](url)",
		"~~strike~~",
		"plain",
		"",
	}
	for _, input := range inputs {
		_ = ParseInlineWithBase(input, p.TableRow, false, p)
		_ = ParseInlineWithBase(input, p.TableHeader, true, p)
	}
}
