//go:build windows

package security

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const cryptProtectUIForbidden uint32 = 0x1

var (
	modCrypt32             = windows.NewLazySystemDLL("crypt32.dll")
	procCryptProtectData   = modCrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = modCrypt32.NewProc("CryptUnprotectData")
)

// dpapiBlobEntropy is a fixed app-specific entropy value to namespace the DPAPI blob.
// Using a fixed entropy prevents other applications from decrypting Argus credentials.
var dpapiBlobEntropy = []byte("Argus/v1/ssh\x00")

type dpapiEncrypter struct{}

func defaultEncrypter() Encrypter { return dpapiEncrypter{} }

type dpBlob struct {
	cbData uint32
	pbData *byte
}

func newDPBlob(b []byte) *dpBlob {
	if len(b) == 0 {
		return &dpBlob{}
	}
	return &dpBlob{cbData: uint32(len(b)), pbData: &b[0]}
}

func (d *dpBlob) bytes() []byte {
	if d.pbData == nil || d.cbData == 0 {
		return nil
	}
	return unsafe.Slice(d.pbData, d.cbData)
}

func (dpapiEncrypter) Encrypt(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return []byte{}, nil
	}
	inBlob := newDPBlob(plaintext)
	entBlob := newDPBlob(dpapiBlobEntropy)
	var outBlob dpBlob

	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(inBlob)),
		0,
		uintptr(unsafe.Pointer(entBlob)),
		0,
		0,
		uintptr(cryptProtectUIForbidden),
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r == 0 {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.pbData))) //nolint:errcheck
	result := make([]byte, outBlob.cbData)
	copy(result, outBlob.bytes())
	return result, nil
}

func (dpapiEncrypter) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 {
		return []byte{}, nil
	}
	inBlob := newDPBlob(ciphertext)
	entBlob := newDPBlob(dpapiBlobEntropy)
	var outBlob dpBlob

	r, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(inBlob)),
		0,
		uintptr(unsafe.Pointer(entBlob)),
		0,
		0,
		uintptr(cryptProtectUIForbidden),
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r == 0 {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.pbData))) //nolint:errcheck
	result := make([]byte, outBlob.cbData)
	copy(result, outBlob.bytes())
	return result, nil
}
