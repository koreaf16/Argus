// Package executor
// File: line_assembler.go
// Description: Chunk-based line assembly with partial-line flush for prompt detection
// Responsibility: Convert raw byte chunks into lines, flushing incomplete lines after a delay

package executor

import (
	"io"
	"strings"
	"time"
)

// PartialFlushDelay is the time to wait before flushing a partial line (no trailing \n).
// Short delay ensures prompts like "password:" are detected quickly.
const PartialFlushDelay = 300 * time.Millisecond

// PipeChunk represents a byte chunk read from a pipe, or a read error.
type PipeChunk struct {
	Data []byte
	Err  error
}

// StartPipeReader reads from r in a goroutine and sends chunks to a channel.
func StartPipeReader(r io.Reader) <-chan PipeChunk {
	ch := make(chan PipeChunk, 8)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				cp := make([]byte, n)
				copy(cp, buf[:n])
				ch <- PipeChunk{Data: cp}
			}
			if readErr != nil {
				ch <- PipeChunk{Err: readErr}
				return
			}
		}
	}()
	return ch
}

// StartLineAssembler converts raw PipeChunks into complete lines.
// Partial lines (e.g., "password:") are flushed after PartialFlushDelay
// to enable prompt detection by the idle handler.
func StartLineAssembler(rawCh <-chan PipeChunk) <-chan string {
	lineCh := make(chan string, 64)
	go func() {
		defer close(lineCh)
		var partial strings.Builder
		timer := time.NewTimer(PartialFlushDelay)
		timer.Stop()

		flushPartial := func() {
			if partial.Len() > 0 {
				lineCh <- partial.String()
				partial.Reset()
			}
		}

		for {
			select {
			case res, ok := <-rawCh:
				if !ok {
					flushPartial()
					return
				}
				if res.Err != nil {
					flushPartial()
					return
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				dataStr := string(res.Data) // Convert to string to process runes
				for _, r := range dataStr {
					if r == '\n' || r == '\r' {
						lineCh <- partial.String()
						partial.Reset()
					} else {
						partial.WriteRune(r)
					}
				}
				if partial.Len() > 0 {
					timer.Reset(PartialFlushDelay)
				}
			case <-timer.C:
				flushPartial()
			}
		}
	}()
	return lineCh
}
