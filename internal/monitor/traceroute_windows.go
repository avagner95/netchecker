// backend/monitor/trace_windows.go
//go:build windows

package monitor

import (
	"bytes"
	"context"
	"errors"
	"netchecker/internal/logging"
	"strings"
	"time"
)

// TraceOnce executes tracert directly (without cmd.exe).
// Even on timeout/cancel it returns partial output in Text.
func TraceOnce(ctx context.Context, address string) TraceOut {
	const hardTimeout = 60 * time.Second

	// Windows tracert tuned defaults:
	// -4   force IPv4
	// -d   don't resolve names
	// -h 20 max hops
	// -w 500 timeout per hop (ms)
	args := []string{
		"-4",
		"-d",
		"-h", "20",
		"-w", "500",
		address,
	}

	tctx, cancel := context.WithTimeout(ctx, hardTimeout)
	defer cancel()

	cmd := execCommandContext(tctx, "tracert", args...)

	// Bound output to avoid memory issues
	const maxOut = 256 * 1024
	var buf bytes.Buffer
	limited := &limitedWriter{w: &buf, n: maxOut}
	cmd.Stdout = limited
	cmd.Stderr = limited

	logging.Info("tracert", "tracert %s", strings.Join(args, " "))

	if err := cmd.Start(); err != nil {
		return TraceOut{OK: false, Err: "spawn", Text: err.Error()}
	}

	err := cmd.Wait()
	out := buf.String()

	// timeout/cancel (keep partial output)
	if errors.Is(tctx.Err(), context.DeadlineExceeded) {
		return TraceOut{OK: false, Err: "timeout", Text: out}
	}
	if errors.Is(tctx.Err(), context.Canceled) {
		return TraceOut{OK: false, Err: "canceled", Text: out}
	}

	// Better success detection
	lower := strings.ToLower(out)

	if strings.Contains(lower, "trace complete") ||
		strings.Contains(lower, "трассировка завершена") {
		return TraceOut{OK: true, Text: out}
	}

	if err != nil {
		return TraceOut{OK: false, Err: "exit", Text: out}
	}

	// If no explicit completion message but process exited cleanly
	return TraceOut{OK: true, Text: out}
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
