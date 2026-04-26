package security

import "errors"

// ErrNotSupported is returned on platforms where DPAPI encryption is unavailable.
var ErrNotSupported = errors.New("native credential encryption is not supported on this platform")

// Encrypter encrypts and decrypts byte slices using OS-provided key storage.
type Encrypter interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// Default returns the platform-native Encrypter.
// Windows: DPAPI (CryptProtectData), bound to the current user account.
// Other: all operations return ErrNotSupported.
func Default() Encrypter {
	return defaultEncrypter()
}
