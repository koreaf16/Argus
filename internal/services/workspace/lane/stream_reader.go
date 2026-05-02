package lane

import (
	"context"
	"errors"
	"io"
	"sync"
)

// streamReader continuously drains a PTY shell's stdout into an internal
// buffer and lets a single waiter block until a Codec sentinel arrives. It
// supports an optional per-call chunk callback that is invoked on the pump
// goroutine for each new burst of bytes received from the shell.
type streamReader struct {
	r    io.Reader
	mu   sync.Mutex
	buf  []byte
	err  error
	cond *sync.Cond

	chunkMu sync.Mutex
	onChunk func(string)
}

func newStreamReader(r io.Reader) *streamReader {
	s := &streamReader{r: r}
	s.cond = sync.NewCond(&s.mu)
	go s.pump()
	return s
}

func (s *streamReader) pump() {
	buf := make([]byte, 4096)
	for {
		n, err := s.r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			s.mu.Lock()
			s.buf = append(s.buf, chunk...)
			s.mu.Unlock()

			s.chunkMu.Lock()
			cb := s.onChunk
			s.chunkMu.Unlock()
			if cb != nil {
				cb(string(chunk))
			}

			s.cond.Broadcast()
		}
		if err != nil {
			s.mu.Lock()
			if s.err == nil {
				s.err = err
			}
			s.mu.Unlock()
			s.cond.Broadcast()
			return
		}
	}
}

// setChunkCallback installs a callback fired on the pump goroutine for every
// new byte burst. Pass nil to clear.
func (s *streamReader) setChunkCallback(cb func(string)) {
	s.chunkMu.Lock()
	s.onChunk = cb
	s.chunkMu.Unlock()
}

// dropAll clears any buffered bytes that arrived before the next command. Used
// after init scripts to discard MOTD / banner noise.
func (s *streamReader) dropAll() {
	s.mu.Lock()
	s.buf = s.buf[:0]
	s.mu.Unlock()
}

// snapshotBuffer returns a copy of the current accumulated buffer. Intended
// for diagnostic logging on priming failure.
func (s *streamReader) snapshotBuffer() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, len(s.buf))
	copy(out, s.buf)
	return out
}

// waitFor blocks until codec finds its sentinel in the accumulated buffer, or
// ctx is cancelled, or the underlying reader closes. Consumed bytes are
// removed from the buffer.
func (s *streamReader) waitFor(ctx context.Context, codec *Codec) (ParsedResult, error) {
	return s.waitForWithMatch(ctx, codec, nil)
}

// promptMatcher inspects accumulated bytes and returns true together with the
// number of bytes to consume up to and including the prompt, allowing the
// caller to react (e.g. inject a password) without consuming the sentinel.
type promptMatcher func(buf []byte) (matched bool, consumeUpTo int)

// waitForWithMatch is waitFor with an additional best-effort prompt matcher.
// When matcher returns true, the matched bytes are reported via promptHits and
// removed from the buffer; the loop continues until the codec sentinel arrives
// or matcher fires N times. matcher fires at most maxPromptHits times to bound
// runaway interactions.
func (s *streamReader) waitForWithMatch(ctx context.Context, codec *Codec, matcher promptMatcher) (ParsedResult, error) {
	const maxPromptHits = 3
	hits := 0

	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-ctx.Done():
			s.cond.Broadcast()
		case <-watchDone:
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		if err := ctx.Err(); err != nil {
			return ParsedResult{}, err
		}
		if res, ok := codec.Parse(s.buf); ok {
			if res.BytesUsed > len(s.buf) {
				s.buf = s.buf[:0]
			} else {
				s.buf = append(s.buf[:0], s.buf[res.BytesUsed:]...)
			}
			return res, nil
		}
		if matcher != nil && hits < maxPromptHits {
			if matched, upTo := matcher(s.buf); matched {
				if upTo > 0 {
					if upTo > len(s.buf) {
						upTo = len(s.buf)
					}
					s.buf = append(s.buf[:0], s.buf[upTo:]...)
				}
				hits++
				return ParsedResult{}, errPromptMatched
			}
		}
		if s.err != nil {
			if errors.Is(s.err, io.EOF) {
				return ParsedResult{}, errors.New("lane: shell closed before sentinel arrived")
			}
			return ParsedResult{}, s.err
		}
		s.cond.Wait()
	}
}

var errPromptMatched = errors.New("lane: prompt matched")
