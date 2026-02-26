//go:build darwin

package monitor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reTTL  = regexp.MustCompile(`\bttl=(\d+)\b`)
	reTime = regexp.MustCompile(`\btime=([0-9.]+)\s*ms\b`)
)

// PingOnce uses system ping (macOS) to ping address once.
func PingOnce(ctx context.Context, address string, timeoutMs int, payload int) PingOut {
	if timeoutMs <= 0 {
		timeoutMs = 1000
	}
	if payload < 0 {
		payload = 0
	}

	// macOS ping:
	// -c 1 (one packet)
	// -W waittime (ms)
	// -s payload bytes
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs+500)*time.Millisecond)
	defer cancel()

	args := []string{"-c", "1", "-W", strconv.Itoa(timeoutMs)}
	if payload > 0 {
		args = append(args, "-s", strconv.Itoa(payload))
	}
	args = append(args, address)

	cmd := exec.CommandContext(cctx, "ping", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	text := out.String()

	// Even on success ping returns 0; on fail non-zero.
	if err != nil {
		// classify quickly
		low := strings.ToLower(text)
		switch {
		case strings.Contains(low, "request timeout"):
			return PingOut{OK: false, Err: "timeout"}
		case strings.Contains(low, "unknown host"):
			return PingOut{OK: false, Err: "dns"}
		case strings.Contains(low, "cannot resolve"):
			return PingOut{OK: false, Err: "dns"}
		default:
			// keep short
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
	m := reTime.FindStringSubmatch(s)
	if len(m) != 2 {
		return nil
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	iv := int(f + 0.5) // round
	return &iv
}

func fmtErr(err error, out string) string {
	if err == nil {
		return ""
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "fail"
	}
	// cap
	if len(out) > 120 {
		out = out[:120]
	}
	return fmt.Sprintf("%v: %s", err, out)
}
