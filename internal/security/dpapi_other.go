//go:build !windows

package security

type unsupportedEncrypter struct{}

func defaultEncrypter() Encrypter { return unsupportedEncrypter{} }

func (unsupportedEncrypter) Encrypt(_ []byte) ([]byte, error) { return nil, ErrNotSupported }
func (unsupportedEncrypter) Decrypt(_ []byte) ([]byte, error) { return nil, ErrNotSupported }
