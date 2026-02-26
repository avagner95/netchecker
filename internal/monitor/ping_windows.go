//go:build windows

package monitor

import (
	"bytes"
	"context"
	"encoding/binary"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

var (
	// Windows output examples:
	// "TTL=57"
	// "time=12ms" OR "time<1ms"
	reTTL  = regexp.MustCompile(`(?i)\bttl[= ](\d+)\b`)
	reTime = regexp.MustCompile(`(?i)\btime\s*([=<])\s*([0-9]+)\s*ms\b`)
)

// PingOnce uses system ping (Windows) to ping address once.
func PingOnce(ctx context.Context, address string, timeoutMs int, payload int) PingOut {
	if timeoutMs <= 0 {
		timeoutMs = 1000
	}
	if payload < 0 {
		payload = 0
	}

	// Windows ping:
	// -n 1        (one packet)
	// -w timeout  (ms)
	// -l payload  (bytes)
	args := []string{"ping", "-n", "1", "-w", strconv.Itoa(timeoutMs)}
	if payload > 0 {
		args = append(args, "-l", strconv.Itoa(payload))
	}
	args = append(args, address)

	// IMPORTANT:
	// Use cmd.exe + chcp 437
	// Use /u to make output UTF-16LE (stable decode), then parse text
	cmdLine := "chcp 437>nul & " + strings.Join(args, " ")

	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs+500)*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(cctx, "cmd.exe", "/u", "/c", cmdLine)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	text := decodeCmdUnicode(out.Bytes())

	// Even on success ping returns 0; on fail non-zero.
	if err != nil {
		low := strings.ToLower(text)
		switch {
		case strings.Contains(low, "request timed out"),
			strings.Contains(low, "превышен интервал ожидания"),
			strings.Contains(low, "превышено время ожидания"):
			return PingOut{OK: false, Err: "timeout"}

		case strings.Contains(low, "could not find host"),
			strings.Contains(low, "не удается разрешить"),
			strings.Contains(low, "unknown host"):
			return PingOut{OK: false, Err: "dns"}

		default:
			return PingOut{OK: false, Err: "fail"}
		}
	}

	ttl := parseTTL(text)
	rtt := parseRTTms(text)
	return PingOut{OK: true, TTL: ttl, RTTms: rtt}
}

func parseTTL(s string) *int {
	m := reTTL.FindStringSubmatch(s)
	if len(m) != 2 {
		return nil
	}
	v, err := strconv.Atoi(m[1])
	if err != nil {
		return nil
	}
	return &v
}

func parseRTTms(s string) *int {
	// time=12ms OR time<1ms
	m := reTime.FindStringSubmatch(s)
	if len(m) != 3 {
		return nil
	}
	op := m[1] // "=" or "<"
	v, err := strconv.Atoi(m[2])
	if err != nil {
		return nil
	}

	// Make it behave like darwin's integer RTT:
	// - time<1ms -> round to 1ms (instead of nil)
	if op == "<" {
		if v <= 1 {
			v = 1
		}
	}

	return &v
}

// cmd.exe /u outputs UTF-16LE. Decode it safely.
// Works regardless of current OEM codepage (chcp), so parsing stays stable.
func decodeCmdUnicode(b []byte) string {
	if len(b) == 0 {
		return ""
	}
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
