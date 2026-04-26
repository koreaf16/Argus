package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/koreaf16/argus/internal/tui"
)

func TestLoadUISettingsMissingFileUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	got := loadUISettings(path)
	want := tui.DefaultUISettings()

	if got != want {
		t.Fatalf("expected defaults %+v, got %+v", want, got)
	}
}

func TestLoadUISettingsParsesAndNormalizes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	data := `{
  "ui": {
    "theme": "classic",
    "variant": "minimal-pro",
    "motion": {
      "enabled": false,
      "level": "expressive",
      "tick_ms": 5,
      "reduced": true,
      "signature": false
    },
    "streaming": {
      "mode": "line-stable",
      "hide_unstable_markdown_tail": false,
      "flush_plain_text_partial": false,
      "render_code_blocks_stable": false
    }
  }
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got := loadUISettings(path)

	if got.Theme != "classic" {
		t.Fatalf("expected theme classic, got %q", got.Theme)
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
	if got.Motion.TickMS != 20 {
		t.Fatalf("expected motion.tick_ms clamped to 20, got %d", got.Motion.TickMS)
	}
	if !got.Motion.Reduced {
		t.Fatalf("expected motion.reduced=true")
	}
	if got.Motion.Signature {
		t.Fatalf("expected motion.signature=false")
	}
	if got.Streaming.Mode != "line-stable" {
		t.Fatalf("expected streaming.mode line-stable, got %q", got.Streaming.Mode)
	}
	if got.Streaming.HideUnstableMarkdown {
		t.Fatalf("expected hide_unstable_markdown_tail=false")
	}
	if got.Streaming.FlushPlainTextPartial {
		t.Fatalf("expected flush_plain_text_partial=false")
	}
	if got.Streaming.RenderCodeBlocksStable {
		t.Fatalf("expected render_code_blocks_stable=false")
	}
}

func TestLoadUISettingsInvalidValuesFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	data := `{
  "ui": {
    "theme": "   ",
    "variant": "neon-x",
    "motion": {
      "level": "ultra-fast",
      "tick_ms": 5000
    },
    "streaming": {
      "mode": "unstable"
    }
  }
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	got := loadUISettings(path)

	if got.Theme != tui.DefaultUITheme {
		t.Fatalf("expected default theme %q, got %q", tui.DefaultUITheme, got.Theme)
	}
	if got.Variant != tui.DefaultUIVariant {
		t.Fatalf("expected default variant %q, got %q", tui.DefaultUIVariant, got.Variant)
	}
	if got.Motion.Level != tui.DefaultMotionLevel {
		t.Fatalf("expected default motion.level %q, got %q", tui.DefaultMotionLevel, got.Motion.Level)
	}
	if got.Motion.TickMS != 1000 {
		t.Fatalf("expected motion.tick_ms clamped to 1000, got %d", got.Motion.TickMS)
	}
	if got.Streaming.Mode != tui.DefaultStreamingMode {
		t.Fatalf("expected default streaming.mode %q, got %q", tui.DefaultStreamingMode, got.Streaming.Mode)
	}
}

