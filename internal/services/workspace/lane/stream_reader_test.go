package lane

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// chunkedReader implements io.Reader by emitting predefined chunks separated
// by an artificial delay. It returns io.EOF after all chunks are consumed.
type chunkedReader struct {
	chunks chan []byte
	closed chan struct{}
}

func newChunkedReader() *chunkedReader {
	return &chunkedReader{
		chunks: make(chan []byte, 16),
		closed: make(chan struct{}),
	}
}

func (c *chunkedReader) Read(p []byte) (int, error) {
	select {
	case <-c.closed:
		return 0, io.EOF
	case chunk, ok := <-c.chunks:
		if !ok {
			return 0, io.EOF
		}
		n := copy(p, chunk)
		return n, nil
	}
}

func (c *chunkedReader) push(s string) {
	c.chunks <- []byte(s)
}

func (c *chunkedReader) close() {
	close(c.chunks)
}

func TestStreamReaderFindsSentinel(t *testing.T) {
	r := newChunkedReader()
	sr := newStreamReader(r)
	defer r.close()

	codec := &Codec{Nonce: "x"}
	go func() {
		r.push("hello\n")
		r.push("world\n")
		r.push(SentinelPrefix + "x:0:/tmp:bob\n")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err := sr.waitFor(ctx, codec)
	if err != nil {
		t.Fatalf("waitFor: %v", err)
	}
	if !strings.Contains(res.Stdout, "hello") || !strings.Contains(res.Stdout, "world") {
		t.Errorf("stdout missing data: %q", res.Stdout)
	}
	if res.User != "bob" || res.CWD != "/tmp" || res.Code != 0 {
		t.Errorf("unexpected meta: %+v", res)
	}
}

func TestStreamReaderSplitSentinel(t *testing.T) {
	r := newChunkedReader()
	sr := newStreamReader(r)
	defer r.close()

	codec := &Codec{Nonce: "n"}
	full := SentinelPrefix + "n:0:/home/u:u\n"
	go func() {
		r.push("output\n")
		r.push(full[:5])
		time.Sleep(20 * time.Millisecond)
		r.push(full[5:])
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err := sr.waitFor(ctx, codec)
	if err != nil {
		t.Fatalf("waitFor: %v", err)
	}
	if res.User != "u" || res.CWD != "/home/u" {
		t.Errorf("split sentinel wrong meta: %+v", res)
	}
}

func TestStreamReaderContextCancel(t *testing.T) {
	r := newChunkedReader()
	sr := newStreamReader(r)
	defer r.close()

	codec := &Codec{Nonce: "z"}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := sr.waitFor(ctx, codec)
	if err == nil {
		t.Fatal("waitFor must error on context cancel")
	}
}

func TestStreamReaderChunkCallback(t *testing.T) {
	r := newChunkedReader()
	sr := newStreamReader(r)
	defer r.close()

	var mu sync.Mutex
	var seen []string
	sr.setChunkCallback(func(c string) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, c)
	})

	codec := &Codec{Nonce: "c"}
	go func() {
		r.push("alpha")
		r.push("beta")
		r.push(SentinelPrefix + "c:0:/x:y\n")
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := sr.waitFor(ctx, codec); err != nil {
		t.Fatalf("waitFor: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 2 {
		t.Fatalf("expected >=2 chunks, got %v", seen)
	}
	if seen[0] != "alpha" || seen[1] != "beta" {
		t.Errorf("chunks = %v", seen)
	}
}

func TestStreamReaderConsumesBytes(t *testing.T) {
	r := newChunkedReader()
	sr := newStreamReader(r)
	defer r.close()

	codec1 := &Codec{Nonce: "first"}
	codec2 := &Codec{Nonce: "second"}
	go func() {
		r.push("one\n" + SentinelPrefix + "first:0:/a:u\n")
		time.Sleep(20 * time.Millisecond)
		r.push("two\n" + SentinelPrefix + "second:0:/b:u\n")
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res1, err := sr.waitFor(ctx, codec1)
	if err != nil {
		t.Fatalf("first waitFor: %v", err)
	}
	if !strings.Contains(res1.Stdout, "one") {
		t.Errorf("res1 stdout = %q", res1.Stdout)
	}

	res2, err := sr.waitFor(ctx, codec2)
	if err != nil {
		t.Fatalf("second waitFor: %v", err)
	}
	if !strings.Contains(res2.Stdout, "two") {
		t.Errorf("res2 stdout = %q", res2.Stdout)
	}
	if strings.Contains(res2.Stdout, "one") {
		t.Errorf("first command output leaked into second: %q", res2.Stdout)
	}
}
