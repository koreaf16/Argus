package lsp

import (
	"bufio"
	"bytes"
	"fmt"
	"testing"
)

func TestReadFramedMessage(t *testing.T) {
	t.Parallel()
	payload := `{"jsonrpc":"2.0","id":1,"result":{}}`
	raw := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)
	r := bufio.NewReader(bytes.NewBufferString(raw))
	body, err := readFramedMessage(r)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if string(body) != payload {
		t.Fatalf("unexpected body: %s", string(body))
	}
}
