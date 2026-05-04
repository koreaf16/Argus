package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/koreaf16/argus/internal/tui"
)

func TestLoadUISettingsMissingFileUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	got := tui.LoadUISettings(path)
	if got.Theme != tui.DefaultUITheme {
		t.Fatalf("expected default theme, got %q", got.Theme)
	}
}

func TestLoadUISettingsParsesAndNormalizes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	data := `{
  "ui": {
    "theme": "dark",
    "variant": "minimal-pro",
    "motion": {
      "enabled": false,
      "level": "expressive",
      "tick_ms": 50,
      "reduced": true
    },
    "streaming": {
      "mode": "TOKEN-LIVE",
      "hide_unstable_markdown_tail": false,
      "render_code_blocks_stable": false
    }
  }
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got := tui.LoadUISettings(path)

	if got.Theme != "dark" {
		t.Fatalf("expected theme dark, got %q", got.Theme)
	}
	if got.Variant != "minimal-pro" {
		t.Fatalf("expected variant minimal-pro, got %q", got.Variant)
	}
	if got.Motion.Enabled {
		t.Fatalf("expected motion.enabled=false")
	}
	if got.Motion.Level != "expressive" {
		t.Fatalf("expected motion.level expressive, got %q", got.Motion.Level)
	}
	if got.Motion.TickMS != 50 {
		t.Fatalf("expected motion.tick_ms=50, got %d", got.Motion.TickMS)
	}
	if !got.Motion.Reduced {
		t.Fatalf("expected motion.reduced=true")
	}
	if got.Streaming.Mode != "token-live" {
		t.Fatalf("expected streaming.mode token-live, got %q", got.Streaming.Mode)
	}
	if got.Streaming.HideUnstableMarkdown {
		t.Fatalf("expected hide_unstable_markdown=false")
	}
	if got.Streaming.RenderCodeBlocksStable {
		t.Fatalf("expected render_code_blocks_stable=false")
	}
}

func TestLoadUISettingsViewThinking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	data := `{
  "ui": {
    "view_thinking": true
  }
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got := tui.LoadUISettings(path)

	if !got.ViewThinking {
		t.Fatalf("expected view_thinking=true")
	}
}

func TestLoadUISettingsInvalidValuesFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	data := `{
  "ui": {
    "motion": {
      "level": "INVALID",
      "tick_ms": 5000
    },
    "streaming": {
      "mode": "UNKNOWN"
    }
  }
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got := tui.LoadUISettings(path)

	if got.Motion.Level != tui.DefaultMotionLevel {
		t.Fatalf("expected default motion level, got %q", got.Motion.Level)
	}
	if got.Motion.TickMS != 1000 {
		t.Fatalf("expected motion.tick_ms clamped to 1000, got %d", got.Motion.TickMS)
	}
	if got.Streaming.Mode != tui.DefaultStreamingMode {
		t.Fatalf("expected default streaming.mode %q, got %q", tui.DefaultStreamingMode, got.Streaming.Mode)
	}
}
