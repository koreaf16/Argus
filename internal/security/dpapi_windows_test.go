//go:build windows

package security

import (
	"bytes"
	"testing"
)

func TestDPAPIRoundtrip(t *testing.T) {
	enc := Default()
	cases := []string{
		"hello",
		"p@ssw0rd!#$%",
		"한글 비밀번호 테스트",
		"very long password: " + string(make([]byte, 512)),
	}
	for _, tc := range cases {
		blob, err := enc.Encrypt([]byte(tc))
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", tc, err)
		}
		plain, err := enc.Decrypt(blob)
		if err != nil {
			t.Fatalf("Decrypt(%q): %v", tc, err)
		}
		if !bytes.Equal(plain, []byte(tc)) {
			t.Fatalf("roundtrip mismatch for %q: got %q", tc, plain)
		}
	}
}

func TestDPAPIEmpty(t *testing.T) {
	enc := Default()
	blob, err := enc.Encrypt([]byte{})
	if err != nil {
		t.Fatal(err)
	}
	if len(blob) != 0 {
		t.Fatalf("expected empty blob, got %d bytes", len(blob))
	}
}

func TestDPAPIDecryptInvalid(t *testing.T) {
	enc := Default()
	_, err := enc.Decrypt([]byte("not a valid dpapi blob"))
	if err == nil {
		t.Fatal("expected error decrypting invalid ciphertext, got nil")
	}
}
