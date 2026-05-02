package lane

import (
	"strings"
	"testing"
)

func TestSentinelFilter(t *testing.T) {
	sentinel := "__SENTINEL__:nonce:"
	
	tests := []struct {
		name     string
		chunks   []string
		expected string
	}{
		{
			name:     "SimpleNoSentinel",
			chunks:   []string{"hello ", "world"},
			expected: "hello world",
		},
		{
			name:     "FullSentinelInOneChunk",
			chunks:   []string{"output", sentinel + "meta\n"},
			expected: "output",
		},
		{
			name:     "SplitSentinelBetweenChunks",
			chunks:   []string{"output" + sentinel[:5], sentinel[5:] + "meta\n"},
			expected: "output",
		},
		{
			name:     "SplitSentinelAcrossMultipleChunks",
			chunks:   []string{"output", "__SENTINEL", "__:non", "ce:", "meta\n"},
			expected: "output",
		},
		{
			name:     "PartialMatchThenMisaligned",
			chunks:   []string{"output__SENT", "X"},
			expected: "output__SENTX",
		},
		{
			name:     "MultipleSentinels",
			chunks:   []string{"first", sentinel, "second", sentinel},
			expected: "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen []string
			filter := NewSentinelFilter(func(c string) {
				seen = append(seen, c)
			}, sentinel)

			for _, chunk := range tt.chunks {
				filter.OnChunk(chunk)
			}

			result := strings.Join(seen, "")
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
