package ssh

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

type fakeWriteCloser struct {
	bytes.Buffer
	closed bool
}

func (f *fakeWriteCloser) Close() error {
	f.closed = true
	return nil
}

func TestSSHSessionSendEOFClosesStdin(t *testing.T) {
	pipe := &fakeWriteCloser{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess := &sshSession{
		ctx:   ctx,
		stdin: pipe,
		done:  make(chan struct{}),
	}

	if err := sess.SendEOF(); err != nil {
		t.Fatalf("SendEOF() error = %v", err)
	}
	if !pipe.closed {
		t.Fatal("expected SendEOF to close stdin pipe")
	}
}

func TestSSHSessionSendEOFFailsWithoutStdin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess := &sshSession{
		ctx:  ctx,
		done: make(chan struct{}),
	}

	if err := sess.SendEOF(); err == nil {
		t.Fatal("expected error when stdin is nil")
	}
}

func TestSSHSessionInjectStdinWritesLine(t *testing.T) {
	pipe := &fakeWriteCloser{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess := &sshSession{
		ctx:   ctx,
		stdin: pipe,
		done:  make(chan struct{}),
	}

	if err := sess.InjectStdin("hello"); err != nil {
		t.Fatalf("InjectStdin() error = %v", err)
	}
	if got := pipe.String(); got != "hello\n" {
		t.Fatalf("expected stdin to contain %q, got %q", "hello\n", got)
	}
}

// TestSSHQuote verifies that sshQuote produces a valid POSIX single-quoted string
// that survives one round of shell expansion, preserving all special characters.
func TestSSHQuote(t *testing.T) {
	cases := []struct {
		input string
		// want is what a POSIX shell would expand the quoted string to.
		// For sshQuote the shell-expanded value must equal input exactly.
		desc string
	}{
		{input: "simple", desc: "plain string"},
		{input: "it's a test", desc: "embedded single quote"},
		{input: "sandbox ALL=(ALL) NOPASSWD: ALL", desc: "sudoers line with parens"},
		{input: "Welcome1!@#$", desc: "special chars: !, @, #, $"},
		{input: "path/to/'dir'/file", desc: "path with single quotes"},
		{input: "a'b'c", desc: "multiple single quotes"},
	}

	for _, tc := range cases {
		quoted := sshQuote(tc.input)

		// The quoted string must start and end with '
		if !strings.HasPrefix(quoted, "'") || !strings.HasSuffix(quoted, "'") {
			t.Errorf("[%s] sshQuote(%q) = %q: must start and end with single quote", tc.desc, tc.input, quoted)
			continue
		}

		// Verify the quoting is consistent: the quoted form should not contain
		// unescaped single quotes that would truncate the shell argument.
		// We do this by checking the round-trip via our own mini-parser that
		// simulates POSIX single-quote expansion rules.
		got, err := posixExpandSingleQuoted(quoted)
		if err != nil {
			t.Errorf("[%s] sshQuote(%q) = %q: shell expansion error: %v", tc.desc, tc.input, quoted, err)
			continue
		}
		if got != tc.input {
			t.Errorf("[%s] sshQuote(%q) round-trip mismatch:\n  quoted: %q\n  got:    %q\n  want:   %q",
				tc.desc, tc.input, quoted, got, tc.input)
		}
	}
}

// posixExpandSingleQuoted simulates POSIX shell expansion of a command-line
// token that may mix single-quoted segments, double-quoted literal apostrophes
// ('"'"' idiom), and unquoted backslash-escaped characters.
// It returns the final concatenated string value.
func posixExpandSingleQuoted(token string) (string, error) {
	var out strings.Builder
	i := 0
	for i < len(token) {
		ch := token[i]
		switch ch {
		case '\'':
			// single-quoted segment: read until matching '
			i++
			for i < len(token) && token[i] != '\'' {
				out.WriteByte(token[i])
				i++
			}
			if i >= len(token) {
				return "", fmt.Errorf("unterminated single-quote in %q", token)
			}
			i++ // consume closing '
		case '"':
			// double-quoted segment: only ' is meaningful here ('"'"' idiom)
			i++
			for i < len(token) && token[i] != '"' {
				out.WriteByte(token[i])
				i++
			}
			if i >= len(token) {
				return "", fmt.Errorf("unterminated double-quote in %q", token)
			}
			i++ // consume closing "
		case '\\':
			// backslash-escape: next char is literal
			i++
			if i >= len(token) {
				return "", fmt.Errorf("trailing backslash in %q", token)
			}
			out.WriteByte(token[i])
			i++
		default:
			out.WriteByte(ch)
			i++
		}
	}
	return out.String(), nil
}
