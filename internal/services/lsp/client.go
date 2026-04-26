package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type Client struct {
	in        io.Writer
	out       io.Reader
	seq       int64
	mu        sync.Mutex
	pending   map[int64]chan Response
	diagMu    sync.RWMutex
	diagCache map[string][]Diagnostic
	closed    chan struct{}
}

func NewClient(in io.Writer, out io.Reader) *Client {
	c := &Client{
		in:        in,
		out:       out,
		pending:   make(map[int64]chan Response),
		diagCache: make(map[string][]Diagnostic),
		closed:    make(chan struct{}),
	}
	go c.readLoop()
	return c
}

func (c *Client) Close() {
	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
}

func (c *Client) Notify(method string, params any) error {
	msg := Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(msg)
}

func (c *Client) Call(method string, params any, out any) error {
	id := atomic.AddInt64(&c.seq, 1)
	msg := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	respCh := make(chan Response, 1)
	c.mu.Lock()
	c.pending[id] = respCh
	c.mu.Unlock()

	if err := c.writeMessage(msg); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return err
	}

	select {
	case <-c.closed:
		return fmt.Errorf("lsp client closed")
	case resp := <-respCh:
		if resp.Error != nil {
			return fmt.Errorf("lsp error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		if out == nil {
			return nil
		}
		b, err := json.Marshal(resp.Result)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, out)
	}
}

func (c *Client) Diagnostics(uri string) []Diagnostic {
	c.diagMu.RLock()
	defer c.diagMu.RUnlock()
	src := c.diagCache[uri]
	out := make([]Diagnostic, len(src))
	copy(out, src)
	return out
}

func (c *Client) DiagnosticsMap() map[string][]Diagnostic {
	c.diagMu.RLock()
	defer c.diagMu.RUnlock()
	out := make(map[string][]Diagnostic, len(c.diagCache))
	for k, v := range c.diagCache {
		cp := make([]Diagnostic, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

func (c *Client) readLoop() {
	r := bufio.NewReader(c.out)
	for {
		select {
		case <-c.closed:
			return
		default:
		}
		body, err := readFramedMessage(r)
		if err != nil {
			c.Close()
			return
		}
		var resp Response
		if err := json.Unmarshal(body, &resp); err != nil {
			continue
		}
		if resp.Method == "textDocument/publishDiagnostics" {
			var params PublishDiagnosticsParams
			raw, _ := json.Marshal(resp.Params)
			if err := json.Unmarshal(raw, &params); err == nil {
				c.diagMu.Lock()
				cp := make([]Diagnostic, len(params.Diagnostics))
				copy(cp, params.Diagnostics)
				c.diagCache[params.URI] = cp
				c.diagMu.Unlock()
			}
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		if ok {
			delete(c.pending, resp.ID)
		}
		c.mu.Unlock()
		if ok {
			ch <- resp
		}
	}
}

func (c *Client) writeMessage(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	header := []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body)))
	buf := bytes.NewBuffer(make([]byte, 0, len(header)+len(body)))
	buf.Write(header)
	buf.Write(body)
	_, err = c.in.Write(buf.Bytes())
	return err
}

func readFramedMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(parts[0]))
		val := strings.TrimSpace(parts[1])
		if key == "content-length" {
			n, err := strconv.Atoi(val)
			if err != nil {
				return nil, err
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}
