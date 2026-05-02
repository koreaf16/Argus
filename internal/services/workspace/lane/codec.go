package lane

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
)

// LaneInitScript is sent to a fresh PTY shell to silence echo and prompts so
// the sentinel-based output capture protocol can rely on clean stdout boundaries.
// The first two `set +o` lines disable bash readline editing so control bytes
// embedded in subsequent commands are not interpreted as keystrokes (the
// historic "Ctrl+A in sentinel ate the printf" bug). The unset -f line removes
// distro hooks like Fedora/Rocky's command_not_found_handle, which otherwise
// emits an interactive "Install package ...? [N/y]" prompt and stalls the lane
// when a script references a missing binary (e.g. docker on a stock host).
const LaneInitScript = "set +o emacs 2>/dev/null || true\n" +
	"set +o vi 2>/dev/null || true\n" +
	"set +H 2>/dev/null || true\n" +
	"unset -f command_not_found_handle 2>/dev/null || true\n" +
	"stty -echo 2>/dev/null || true\n" +
	"PS1='' PS2='' PROMPT_COMMAND=''\n" +
	"export PS1 PS2 PROMPT_COMMAND\n" +
	"export LC_ALL=C LANG=C\n"

// SentinelPrefix is emitted by the trailing printf after every user command.
// We use printable ASCII only — the per-call random nonce (8 hex bytes, 2^32
// space) supplies the uniqueness that prevents accidental matches in user
// output. Earlier versions used a leading SOH (\x01) byte, but bash readline
// in interactive PTYs interprets Ctrl+A as "beginning-of-line" and silently
// shreds the priming command, causing 30s timeouts on every connect.
const SentinelPrefix = "__ARGUS_LANE_END__:"

// Codec wraps a user command with a trailing sentinel echo that conveys the
// exit code, cwd, and effective user. The parser scans accumulated stream
// bytes for a line starting with SentinelPrefix and matching the codec's nonce.
type Codec struct {
	Nonce string
}

// NewCodec generates a fresh nonce for a single Exec call.
func NewCodec() *Codec {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return &Codec{Nonce: hex.EncodeToString(b[:])}
}

// Wrap returns the bytes to write to the shell stdin: the user command, a
// newline, then a single printf line that emits the sentinel with metadata.
// The wrapper does not introduce its own quoting around the user command, so
// callers can pass arbitrary multi-line scripts including heredocs.
func (c *Codec) Wrap(cmd string) string {
	cmd = strings.TrimRight(cmd, "\n")
	return cmd + "\n" +
		"__arg_ec=$?\n" +
		`printf '` + SentinelPrefix + c.Nonce + `:%d:%s:%s\n' "$__arg_ec" "$(pwd)" "$(id -un 2>/dev/null || echo unknown)"` + "\n"
}

// ParsedResult is what Codec.Parse returns once the sentinel for the matching
// nonce is found. BytesUsed indicates how many bytes of the input stream were
// consumed (caller should drop them from its accumulator).
type ParsedResult struct {
	Stdout    string
	Code      int
	CWD       string
	User      string
	BytesUsed int
}

// Parse scans stream for the sentinel line "<SentinelPrefix><Nonce>:<code>:<cwd>:<user>\n".
// Returns (result, true) when found; otherwise (zero, false) and the caller
// should buffer more bytes.
func (c *Codec) Parse(stream []byte) (ParsedResult, bool) {
	target := []byte(SentinelPrefix + c.Nonce + ":")
	idx := bytes.Index(stream, target)
	if idx < 0 {
		return ParsedResult{}, false
	}
	end := bytes.IndexByte(stream[idx:], '\n')
	if end < 0 {
		return ParsedResult{}, false
	}
	line := string(stream[idx+len(target) : idx+end])
	parts := strings.SplitN(line, ":", 3)
	if len(parts) != 3 {
		return ParsedResult{}, false
	}
	code, err := strconv.Atoi(parts[0])
	if err != nil {
		return ParsedResult{}, false
	}
	stdout := string(stream[:idx])
	stdout = strings.TrimRight(stdout, "\r\n")
	return ParsedResult{
		Stdout:    stdout,
		Code:      code,
		CWD:       parts[1],
		User:      parts[2],
		BytesUsed: idx + end + 1,
	}, true
}

// SentinelFilter wraps a streaming chunk callback to suppress the sentinel
// marker and metadata that follows it. It handles cases where the sentinel
// string is split across multiple chunk bursts.
type SentinelFilter struct {
	cb       func(string)
	sentinel string
	buffer   string
	stopped  bool
}

// NewSentinelFilter creates a filter that suppresses the given sentinel string
// from the callback's output.
func NewSentinelFilter(cb func(string), sentinel string) *SentinelFilter {
	return &SentinelFilter{
		cb:       cb,
		sentinel: sentinel,
	}
}

func (f *SentinelFilter) OnChunk(chunk string) {
	if f.stopped {
		return
	}

	// Append new chunk to tail of previous (possibly partial) match
	combined := f.buffer + chunk

	// If we found the full sentinel, emit only what came before it and stop.
	if idx := strings.Index(combined, f.sentinel); idx >= 0 {
		f.cb(combined[:idx])
		f.stopped = true
		f.buffer = ""
		return
	}

	// If we didn't find the full sentinel, we might have a partial match at
	// the end of the buffer. We need to hold back anything that looks like
	// the start of the sentinel until we see more bytes.
	keepLen := len(combined)
	for i := 1; i < len(f.sentinel) && i <= len(combined); i++ {
		// Does the end of combined match the start of the sentinel?
		if strings.HasSuffix(combined, f.sentinel[:i]) {
			keepLen = len(combined) - i
			break
		}
	}

	if keepLen > 0 {
		f.cb(combined[:keepLen])
		f.buffer = combined[keepLen:]
	} else {
		f.buffer = combined
	}
}
