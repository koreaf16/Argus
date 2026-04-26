package tui

import "strings"

const (
	DefaultUITheme   = "default"
	DefaultUIVariant = "argus-signature"

	DefaultMotionLevel = "restrained"
	DefaultMotionTick  = 100

	DefaultStreamingMode = "line-stable"
)

type MotionSettings struct {
	Enabled   bool   `json:"enabled"`
	Level     string `json:"level"`
	TickMS    int    `json:"tick_ms"`
	Reduced   bool   `json:"reduced"`
	Signature bool   `json:"signature"`
}

type StreamingSettings struct {
	Mode                   string `json:"mode"`
	HideUnstableMarkdown   bool   `json:"hide_unstable_markdown_tail"`
	FlushPlainTextPartial  bool   `json:"flush_plain_text_partial"`
	RenderCodeBlocksStable bool   `json:"render_code_blocks_stable"`
}

type UISettings struct {
	Theme     string            `json:"theme"`
	Variant   string            `json:"variant"`
	Motion    MotionSettings    `json:"motion"`
	Streaming StreamingSettings `json:"streaming"`
}

func DefaultUISettings() UISettings {
	return UISettings{
		Theme:   DefaultUITheme,
		Variant: DefaultUIVariant,
		Motion: MotionSettings{
			Enabled:   true,
			Level:     DefaultMotionLevel,
			TickMS:    DefaultMotionTick,
			Reduced:   false,
			Signature: true,
		},
		Streaming: StreamingSettings{
			Mode:                   DefaultStreamingMode,
			HideUnstableMarkdown:   true,
			FlushPlainTextPartial:  true,
			RenderCodeBlocksStable: true,
		},
	}
}

func ResolveUISettings(in UISettings, fallbackTheme string, aiDebug bool) UISettings {
	out := DefaultUISettings()
	if strings.TrimSpace(fallbackTheme) != "" {
		out.Theme = strings.TrimSpace(fallbackTheme)
	}
	if strings.TrimSpace(in.Theme) != "" {
		out.Theme = strings.TrimSpace(in.Theme)
	}
	if strings.TrimSpace(in.Variant) != "" {
		out.Variant = strings.TrimSpace(in.Variant)
	}
	if !isMotionFieldUnset(in.Motion) {
		out.Motion = in.Motion
	}
	if in.Motion.Level != "" {
		out.Motion.Level = strings.ToLower(strings.TrimSpace(in.Motion.Level))
	}
	if in.Motion.TickMS > 0 {
		out.Motion.TickMS = in.Motion.TickMS
	}

	if !isStreamingFieldUnset(in.Streaming) {
		out.Streaming = in.Streaming
	}
	if strings.TrimSpace(in.Streaming.Mode) != "" {
		out.Streaming.Mode = strings.ToLower(strings.TrimSpace(in.Streaming.Mode))
	}

	if out.Motion.TickMS < 20 {
		out.Motion.TickMS = 20
	}
	if out.Motion.TickMS > 1000 {
		out.Motion.TickMS = 1000
	}
	switch out.Motion.Level {
	case "static", "restrained", "expressive":
	default:
		out.Motion.Level = DefaultMotionLevel
	}
	switch strings.ToLower(out.Variant) {
	case "current", "argus-signature", "minimal-pro":
	default:
		out.Variant = DefaultUIVariant
	}
	switch out.Streaming.Mode {
	case "hybrid-stable", "line-stable", "token-live":
	default:
		out.Streaming.Mode = DefaultStreamingMode
	}
	if aiDebug {
		out.Motion.Enabled = false
		out.Motion.Reduced = true
	}
	return out
}

func isMotionFieldUnset(m MotionSettings) bool {
	return !m.Enabled && m.Level == "" && m.TickMS == 0 && !m.Reduced && !m.Signature
}

func isStreamingFieldUnset(s StreamingSettings) bool {
	return s.Mode == "" && !s.HideUnstableMarkdown && !s.FlushPlainTextPartial && !s.RenderCodeBlocksStable
}
