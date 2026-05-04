package tui

import (
	"encoding/json"
	"os"
	"strings"
)

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
	Theme        string            `json:"theme"`
	Variant      string            `json:"variant"`
	ViewThinking bool              `json:"view_thinking"`
	Motion       MotionSettings    `json:"motion"`
	Streaming    StreamingSettings `json:"streaming"`
}

func DefaultUISettings() UISettings {
	return UISettings{
		Theme:        DefaultUITheme,
		Variant:      DefaultUIVariant,
		ViewThinking: false,
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
	out.ViewThinking = in.ViewThinking
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

func LoadUISettings(settingsPath string) UISettings {
	settings := DefaultUISettings()

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return settings
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return settings
	}
	uiRaw, ok := root["ui"]
	if !ok {
		return settings
	}
	uiMap, ok := uiRaw.(map[string]any)
	if !ok {
		return settings
	}

	if v, ok := uiMap["theme"].(string); ok && strings.TrimSpace(v) != "" {
		settings.Theme = strings.TrimSpace(v)
	}
	if v, ok := uiMap["variant"].(string); ok && strings.TrimSpace(v) != "" {
		settings.Variant = strings.TrimSpace(v)
	}
	if v, ok := uiMap["view_thinking"].(bool); ok {
		settings.ViewThinking = v
	}

	if motionRaw, ok := uiMap["motion"].(map[string]any); ok {
		if v, ok := motionRaw["enabled"].(bool); ok {
			settings.Motion.Enabled = v
		}
		if v, ok := motionRaw["level"].(string); ok && strings.TrimSpace(v) != "" {
			settings.Motion.Level = strings.TrimSpace(v)
		}
		if v, ok := motionRaw["tick_ms"]; ok {
			switch n := v.(type) {
			case float64:
				settings.Motion.TickMS = int(n)
			case int:
				settings.Motion.TickMS = n
			}
		}
		if v, ok := motionRaw["reduced"].(bool); ok {
			settings.Motion.Reduced = v
		}
		if v, ok := motionRaw["signature"].(bool); ok {
			settings.Motion.Signature = v
		}
	}

	if streamingRaw, ok := uiMap["streaming"].(map[string]any); ok {
		if v, ok := streamingRaw["mode"].(string); ok && strings.TrimSpace(v) != "" {
			settings.Streaming.Mode = strings.TrimSpace(v)
		}
		if v, ok := streamingRaw["hide_unstable_markdown_tail"].(bool); ok {
			settings.Streaming.HideUnstableMarkdown = v
		}
		if v, ok := streamingRaw["flush_plain_text_partial"].(bool); ok {
			settings.Streaming.FlushPlainTextPartial = v
		}
		if v, ok := streamingRaw["render_code_blocks_stable"].(bool); ok {
			settings.Streaming.RenderCodeBlocksStable = v
		}
	}

	return ResolveUISettings(settings, settings.Theme, false)
}
