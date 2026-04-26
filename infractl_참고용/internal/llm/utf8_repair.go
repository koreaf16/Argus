// Package llm
// File: utf8_repair.go
// Description: SSE 스트림에서 멀티바이트 UTF-8이 청크 경계에서 분할될 때 복구하는 버퍼
// Responsibility: 불완전한 UTF-8 바이트 시퀀스를 청크 경계에 걸쳐 버퍼링하고 완전한 룬만 방출

package llm

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"
)

// utf8RepairBuf buffers raw bytes across SSE chunks, emitting only complete UTF-8 sequences.
// This handles models (e.g. vLLM Gemma) that stream individual bytes of multibyte characters.
type utf8RepairBuf struct {
	b []byte
}

// push appends raw bytes and returns a string of all complete runes.
// Any trailing incomplete multibyte sequence is held in the buffer.
func (u *utf8RepairBuf) push(raw []byte) string {
	u.b = append(u.b, raw...)
	cut := utf8SafeCut(u.b)
	if cut == 0 {
		return ""
	}
	out := string(u.b[:cut])
	u.b = u.b[cut:]
	return out
}

// flush emits all remaining buffered bytes, treating incomplete sequences as-is.
func (u *utf8RepairBuf) flush() string {
	if len(u.b) == 0 {
		return ""
	}
	out := string(u.b)
	u.b = nil
	return out
}

// utf8SafeCut returns the length of the longest complete-rune prefix of b.
// Trailing bytes that form an incomplete multibyte sequence are excluded.
func utf8SafeCut(b []byte) int {
	safe := 0
	for i := 0; i < len(b); {
		if b[i] < 0x80 {
			i++
			safe = i
			continue
		}
		// Bare continuation byte (0x80–0xBF) or overlong lead (0xC0–0xC1) — invalid; advance.
		if b[i] < 0xC2 {
			i++
			safe = i
			continue
		}
		var need int
		switch {
		case b[i] < 0xE0:
			need = 2
		case b[i] < 0xF0:
			need = 3
		default:
			need = 4
		}
		if i+need > len(b) {
			return safe // incomplete sequence at end — hold back
		}
		i += need
		safe = i
	}
	return safe
}

// decodeJSONStringRaw decodes a JSON string literal to raw bytes without UTF-8 validation.
// This preserves raw byte sequences that json.Unmarshal would otherwise replace with U+FFFD.
// raw must be a JSON string literal (surrounded by double-quote characters).
// Returns (nil, false) for non-string tokens such as null, numbers, or booleans.
func decodeJSONStringRaw(raw json.RawMessage) ([]byte, bool) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return nil, false
	}
	inner := raw[1 : len(raw)-1]
	if len(inner) == 0 {
		return []byte{}, true
	}
	out := make([]byte, 0, len(inner))
	for i := 0; i < len(inner); {
		if inner[i] != '\\' {
			out = append(out, inner[i])
			i++
			continue
		}
		if i+1 >= len(inner) {
			return nil, false
		}
		switch inner[i+1] {
		case '"':
			out = append(out, '"')
		case '\\':
			out = append(out, '\\')
		case '/':
			out = append(out, '/')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			if i+5 >= len(inner) {
				return nil, false
			}
			r1val, err := strconv.ParseUint(string(inner[i+2:i+6]), 16, 16)
			if err != nil {
				return nil, false
			}
			r1 := rune(r1val)
			// Surrogate pair: \uD800–\uDBFF followed by \uDC00–\uDFFF.
			if r1 >= 0xD800 && r1 <= 0xDBFF && i+11 < len(inner) &&
				inner[i+6] == '\\' && inner[i+7] == 'u' {
				r2val, err2 := strconv.ParseUint(string(inner[i+8:i+12]), 16, 16)
				if err2 == nil && rune(r2val) >= 0xDC00 && rune(r2val) <= 0xDFFF {
					combined := 0x10000 + (r1-0xD800)*0x400 + (rune(r2val) - 0xDC00)
					out = utf8.AppendRune(out, combined)
					i += 12
					continue
				}
			}
			out = utf8.AppendRune(out, r1)
			i += 6
			continue
		default:
			out = append(out, inner[i+1])
		}
		i += 2
	}
	return out, true
}
