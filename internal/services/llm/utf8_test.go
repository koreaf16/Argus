package llm

import (
	"testing"
)

type mockEmitter struct {
	text     string
	thinking string
}

func (m *mockEmitter) EmitText(s string)     { m.text += s }
func (m *mockEmitter) EmitThinking(s string) { m.thinking += s }
func (m *mockEmitter) EmitStatus(s string)   {}

func TestChannelTokenFilter_UTF8Breaks_InBody(t *testing.T) {
	f := &channelTokenFilter{}
	e := &mockEmitter{}

	// Enter stateChannelBody
	f.feed("<|channel|>final<|message|>", e)
	e.text = "" // clear header emissions if any
	
	// "한" (ED 95 9C) split
	part1 := string([]byte{0xED, 0x95})
	part2 := string([]byte{0x9C})

	f.feed(part1, e)
	// If it emits immediately, e.text will have partial bytes.
	if len(e.text) > 0 {
		t.Logf("Emitted partial bytes: %x", e.text)
	}
	
	f.feed(part2, e)
	f.flush(e)

	expected := "한"
	if e.text != expected {
		t.Errorf("Expected %q, got %q (hex: %x)", expected, e.text, e.text)
	}
}
