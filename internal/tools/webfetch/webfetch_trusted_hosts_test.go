package webfetch

import "testing"

func TestIsBuiltInTrustedHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{host: "ai.google.dev", want: true},
		{host: "sub.ai.google.dev", want: true},
		{host: "huggingface.co", want: true},
		{host: "example.com", want: false},
		{host: "", want: false},
	}

	for _, tc := range cases {
		got := isBuiltInTrustedHost(tc.host)
		if got != tc.want {
			t.Fatalf("host %q: expected %v, got %v", tc.host, tc.want, got)
		}
	}
}
