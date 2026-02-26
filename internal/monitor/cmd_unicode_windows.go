//go:build windows

package monitor

import (
	"encoding/binary"
	"unicode/utf16"
)

// cmd.exe /u outputs UTF-16LE. Decode it safely.
// Works regardless of current OEM codepage (chcp), so parsing/UI stays stable.
func decodeCmdUnicode(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// trim odd byte
	if len(b)%2 == 1 {
		b = b[:len(b)-1]
	}

	u16 := make([]uint16, 0, len(b)/2)
	for i := 0; i < len(b); i += 2 {
		u16 = append(u16, binary.LittleEndian.Uint16(b[i:i+2]))
	}

	// drop BOM if present
	if len(u16) > 0 && u16[0] == 0xFEFF {
		u16 = u16[1:]
	}

	return string(utf16.Decode(u16))
}
