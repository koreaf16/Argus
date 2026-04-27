package llm

import (
	"strings"
	"testing"
)

func TestChannelTokenFilter_StripInlineTokens(t *testing.T) {
	tests := []struct {
		name   string
		chunks []string // fed one at a time
		want   string
	}{
		{
			name:   "double-pipe final channel",
			chunks: []string{"<|channel|>final<|message|>안녕<|end|>"},
			want:   "안녕",
		},
		{
			name:   "right-pipe token with adjacent content",
			chunks: []string{"<channel|>현재 사용 중인 디스크"},
			want:   "현재 사용 중인 디스크",
		},
		{
			name:   "left-pipe prefix token",
			chunks: []string{"<|channel>thought\n본문"},
			want:   "본문",
		},
		{
			name:   "standalone channel label line removed",
			chunks: []string{"hello\nanalysis\nworld\n"},
			want:   "hello\nworld\n",
		},
		{
			name:   "standalone thought label removed",
			chunks: []string{"thought\n"},
			want:   "",
		},
		{
			name:   "plain text unchanged",
			chunks: []string{"Hello world\n"},
			want:   "Hello world\n",
		},
		{
			name:   "token split across chunks",
			chunks: []string{"before <|chan", "nel|> after"},
			want:   "before  after",
		},
		{
			name:   "multiple tokens stripped",
			chunks: []string{"<|channel|>analysis<|message|>thinking<|end|><|channel|>final<|message|>답변<|return|>"},
			want:   "thinking답변",
		},
		{
			name:   "regular less-than not held",
			chunks: []string{"x < 5 is true"},
			want:   "x < 5 is true",
		},
		{
			name:   "control token at line start with content after",
			chunks: []string{"<|message|>Hello\n<|end|>"},
			want:   "Hello\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &channelTokenFilter{}
			var buf strings.Builder
			for _, chunk := range tc.chunks {
				buf.WriteString(f.feed(chunk))
			}
			buf.WriteString(f.flush())
			got := buf.String()
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApplyHarmonyFilter(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"<|channel|>final<|message|>ok<|end|>", "ok"},
		{"<channel|>hello world", "hello world"},
		{"normal text", "normal text"},
		{"line1\nthought\nline3\n", "line1\nline3\n"},
		{"analysis\n", ""},
		{"commentary\nsome text\nfinal\n", "some text\n"},
		{"", ""},
	}
	for _, tc := range tests {
		got := applyHarmonyFilter(tc.in)
		if got != tc.want {
			t.Errorf("applyHarmonyFilter(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSplitAtPotentialToken(t *testing.T) {
	tests := []struct {
		text     string
		wantSafe string
		wantHeld string
	}{
		{"hello <|chann", "hello ", "<|chann"},
		{"hello <channel", "hello ", "<channel"},
		{"hello < world", "hello < world", ""},
		{"<|channel|> done", "<|channel|> done", ""},
		{"no angle bracket", "no angle bracket", ""},
		{"text <|done|> more", "text <|done|> more", ""},
	}
	for _, tc := range tests {
		safe, held := splitAtPotentialToken(tc.text)
		if safe != tc.wantSafe || held != tc.wantHeld {
			t.Errorf("splitAtPotentialToken(%q) = (%q, %q), want (%q, %q)",
				tc.text, safe, held, tc.wantSafe, tc.wantHeld)
		}
	}
}
