package lane

import (
	"context"
	"io"
)

// PromptMatcher is the exported alias of the internal promptMatcher type so
// callers in sibling packages (channel/) can use the prompt-aware wait API
// without re-implementing PTY drain logic.
type PromptMatcher = promptMatcher

// StreamReader is an exported handle around the unexported streamReader. The
// channel package owns the PTY shell stdout pipe and uses this to wait for
// codec sentinels and password prompts.
type StreamReader struct {
	inner *streamReader
}

// NewStreamReader builds a StreamReader that drains r in a background pump.
func NewStreamReader(r io.Reader) *StreamReader {
	return &StreamReader{inner: newStreamReader(r)}
}

// SetChunkCallback installs a per-burst stdout callback. Pass nil to clear.
func (s *StreamReader) SetChunkCallback(cb func(string)) {
	s.inner.setChunkCallback(cb)
}

// DropAll clears any buffered bytes that have not yet been consumed.
func (s *StreamReader) DropAll() {
	s.inner.dropAll()
}

// SnapshotBuffer returns a copy of the current accumulated buffer for
// diagnostic logging.
func (s *StreamReader) SnapshotBuffer() []byte {
	return s.inner.snapshotBuffer()
}

// WaitFor blocks until codec finds its sentinel.
func (s *StreamReader) WaitFor(ctx context.Context, codec *Codec) (ParsedResult, error) {
	return s.inner.waitFor(ctx, codec)
}

// WaitForWithMatch is WaitFor plus an opportunistic prompt matcher. When the
// matcher fires, the returned error is ErrPromptMatched and the matched bytes
// are consumed; callers should respond (e.g. inject a password) and call again.
func (s *StreamReader) WaitForWithMatch(ctx context.Context, codec *Codec, matcher PromptMatcher) (ParsedResult, error) {
	return s.inner.waitForWithMatch(ctx, codec, matcher)
}

// ErrPromptMatched signals that the prompt matcher fired before the codec
// sentinel arrived. The reader has consumed up to and including the prompt.
var ErrPromptMatched = errPromptMatched

// MakePasswordPromptMatcher exports the package's default sudo/su password
// prompt detector so external callers can reuse the regex.
func MakePasswordPromptMatcher() PromptMatcher {
	return makePasswordPromptMatcher()
}
