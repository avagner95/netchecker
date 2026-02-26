// backend/monitor/trace_windows.go
//go:build windows

package monitor

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os/exec"
	"strings"
	"time"
	"unicode/utf16"

	"netchecker/internal/logging"
)

// TraceOnce executes tracert with a hard timeout.
// Even on timeout/cancel it returns partial output in Text.
func TraceOnce(ctx context.Context, address string) TraceOut {
	// You asked for 60 seconds.
	const hardTimeout = 60 * time.Second

	// Windows tracert "quick" defaults:
	// -d   don't resolve names
	// -h 20 max hops
	// -w 500 timeout per hop (ms)
	// Worst-case ~= 20 * 500ms * 3 probes = 30s, but keep 60s hard cap.
	args := []string{"tracert", "-d", "-h", "20", "-w", "500", address}

	tctx, cancel := context.WithTimeout(ctx, hardTimeout)
	defer cancel()

	// Run via cmd with chcp 437, and /u for UTF-16LE output
	cmdLine := "chcp 437>nul & " + strings.Join(args, " ")
	cmd := exec.CommandContext(tctx, "cmd.exe", "/u", "/c", cmdLine)

	// Bound output to avoid memory issues
	const maxOut = 256 * 1024
	var buf bytes.Buffer
	limited := &limitedWriter{w: &buf, n: maxOut}
	cmd.Stdout = limited
	cmd.Stderr = limited

	logging.Info("tracert", "%s %s", "cmd.exe", cmdLine)

	if err := cmd.Start(); err != nil {
		return TraceOut{OK: false, Err: "spawn", Text: err.Error()}
	}

	err := cmd.Wait()

	// decode cmd.exe /u output (UTF-16LE)
	out := decodeCmdUnicode(buf.Bytes())

	// timeout/cancel (keep partial output)
	if errors.Is(tctx.Err(), context.DeadlineExceeded) {
		return TraceOut{OK: false, Err: "timeout", Text: out}
	}
	if errors.Is(tctx.Err(), context.Canceled) {
		return TraceOut{OK: false, Err: "canceled", Text: out}
	}

	// heuristic: if we saw destination IP in output -> treat as OK
	// (works well with -d)
	if strings.Contains(out, address) {
		return TraceOut{OK: true, Err: "", Text: out}
	}

	if err != nil {
		return TraceOut{OK: false, Err: "exit", Text: out}
	}

	return TraceOut{OK: true, Err: "", Text: out}
}

// limitedWriter truncates output after N bytes.
type limitedWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil // drop silently
	}
	if len(p) > l.n {
		p = p[:l.n]
	}
	l.n -= len(p)
	return l.w.Write(p)
}

// cmd.exe /u outputs UTF-16LE. Decode it safely.
// Works regardless of current OEM codepage (chcp), so UI text is stable.
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
