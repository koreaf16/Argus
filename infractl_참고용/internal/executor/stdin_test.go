package executor

import (
	"bytes"
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

func TestLocalExecutorSendEOFClosesPipeMode(t *testing.T) {
	pipe := &fakeWriteCloser{}
	exec := &LocalExecutor{
		activeStdin:     pipe,
		activeStdinMode: stdinModePipe,
	}

	if err := exec.SendEOF(); err != nil {
		t.Fatalf("SendEOF() error = %v", err)
	}
	if !pipe.closed {
		t.Fatal("expected pipe mode SendEOF to close stdin")
	}
	if exec.activeStdin != nil {
		t.Fatal("expected active stdin to be cleared after pipe EOF")
	}
}

func TestLocalExecutorSendEOFWritesCtrlDForPTY(t *testing.T) {
	pipe := &fakeWriteCloser{}
	exec := &LocalExecutor{
		activeStdin:     pipe,
		activeStdinMode: stdinModePTY,
	}

	if err := exec.SendEOF(); err != nil {
		t.Fatalf("SendEOF() error = %v", err)
	}
	if pipe.closed {
		t.Fatal("expected PTY mode SendEOF not to close stdin")
	}
	if got := pipe.Bytes(); len(got) != 1 || got[0] != 0x04 {
		t.Fatalf("expected PTY mode SendEOF to write 0x04, got %v", got)
	}
}
