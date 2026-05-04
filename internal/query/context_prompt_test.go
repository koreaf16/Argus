package query

import (
	"strings"
	"testing"
)

func TestDefaultSystemPrompt_IncludesParallelToolCallGuidance(t *testing.T) {
	t.Parallel()

	blocks := DefaultSystemPrompt()
	if len(blocks) == 0 {
		t.Fatalf("expected system prompt blocks")
	}
	text := blocks[0].Text
	// Korean: "한 번의 응답으로 여러 도구 호출을 생성하여 병렬로 실행될 수 있도록 하십시오."
	if !strings.Contains(text, "병렬") && !strings.Contains(text, "parallel") && !strings.Contains(text, "emit multiple tool calls") {
		t.Fatalf("missing parallel tool call guidance in system prompt")
	}
}

func TestDefaultSystemPrompt_IncludesGreetingGuidance(t *testing.T) {
	t.Parallel()

	blocks := DefaultSystemPrompt()
	if len(blocks) == 0 {
		t.Fatalf("expected system prompt blocks")
	}
	text := blocks[0].Text
	// Korean: "단순한 인사, 잡담, 감정 표현의 경우 도구를 사용하지 않고 직접 텍스트로 응답하십시오."
	if !strings.Contains(text, "인사") && !strings.Contains(text, "greetings") && !strings.Contains(text, "chit-chat") {
		t.Fatalf("missing greeting guidance in system prompt")
	}
}

func TestDefaultSystemPrompt_SplitsDynamicTimeBlock(t *testing.T) {
	t.Parallel()

	blocks := DefaultSystemPrompt()
	if len(blocks) < 2 {
		t.Fatalf("expected static and dynamic system prompt blocks, got %d", len(blocks))
	}
	// Korean time block: "현재 시스템 날짜 및 시간:"
	dynamicMarker := "현재 시스템 날짜 및 시간"
	if strings.Contains(blocks[0].Text, dynamicMarker) {
		t.Fatal("static system prompt block should not contain dynamic time")
	}
	if !strings.Contains(blocks[1].Text, dynamicMarker) {
		t.Fatalf("missing dynamic time block: %+v", blocks[1])
	}
	if blocks[0].CacheControl["type"] != "ephemeral" {
		t.Fatalf("static block should opt into provider cache control: %+v", blocks[0].CacheControl)
	}
}

