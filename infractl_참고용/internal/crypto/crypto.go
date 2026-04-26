// Package crypto
// File: crypto.go
// Description: AES-256-GCM 암호화/복호화 및 머신 고유 키 파생
// Responsibility: 크리덴셜 저장을 위한 암호화 레이어 제공

package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const keySalt = "infractl-credential-key-v1"

// DeriveKey는 이 머신에 고유한 32바이트 AES 키를 파생한다.
// SHA-256(hostname + machineID + salt) 방식으로 생성한다.
// 키는 파일에 저장되지 않으며 런타임에 재파생된다.
func DeriveKey() ([]byte, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("get hostname: %w", err)
	}

	mid, err := machineID()
	if err != nil {
		// machineID 실패 시 hostname만으로 파생 (degraded mode)
		mid = "unknown"
	}

	raw := strings.Join([]string{hostname, mid, keySalt}, "|")
	digest := sha256.Sum256([]byte(raw))
	return digest[:], nil
}

// Encrypt는 key로 plaintext를 AES-256-GCM 암호화하고 base64 문자열을 반환한다.
// nonce는 매 호출마다 새로 생성된다.
func Encrypt(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	// nonce + ciphertext 조합 후 base64 인코딩
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt는 key로 base64 인코딩된 ciphertext를 복호화하고 원문을 반환한다.
func Decrypt(key []byte, ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// machineID는 OS별 머신 고유 식별자를 반환한다.
// Windows: 레지스트리 MachineGuid
// Linux/Mac: /etc/machine-id 또는 /var/lib/dbus/machine-id
func machineID() (string, error) {
	switch runtime.GOOS {
	case "windows":
		return windowsMachineID()
	default:
		return unixMachineID()
	}
}

func windowsMachineID() (string, error) {
	out, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive",
		"-Command", `(Get-ItemProperty "HKLM:\SOFTWARE\Microsoft\Cryptography" MachineGuid).MachineGuid`,
	).Output()
	if err != nil {
		return "", fmt.Errorf("read windows machine guid: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func unixMachineID() (string, error) {
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	}
	return "", fmt.Errorf("machine-id file not found")
}
