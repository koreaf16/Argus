// Package powershell
// File: encode.go
// Description: PowerShell -EncodedCommand 용 UTF-16LE base64 인코더
// Responsibility: EncodeUTF16LE — 한글·특수문자 포함 스크립트 안전 전달

// Ported from: claude_cli/src/utils/shell/powershellProvider.ts:encodePowerShellCommand

package powershell

import (
	"encoding/base64"
	"encoding/binary"
	"unicode/utf16"
)

// EncodeUTF16LE 는 PowerShell -EncodedCommand 플래그용 UTF-16LE base64 문자열을 반환한다.
// PowerShell 은 -EncodedCommand 인자를 UTF-16LE(리틀엔디언) base64 로 디코딩한다.
// Ported from: claude_cli/src/utils/shell/powershellProvider.ts:encodePowerShellCommand
func EncodeUTF16LE(script string) string {
	runes := utf16.Encode([]rune(script))
	buf := make([]byte, len(runes)*2)
	for i, r := range runes {
		binary.LittleEndian.PutUint16(buf[i*2:], r)
	}
	return base64.StdEncoding.EncodeToString(buf)
}
