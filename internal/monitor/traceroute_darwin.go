// backend/monitor/trace_darwin.go
//go:build darwin

package monitor

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"netchecker/internal/logging"
)

type TraceOut struct {
	OK   bool
	Text string
	Err  string // "timeout" | "canceled" | "exit" | "spawn"
}

// TraceOnce executes /usr/sbin/traceroute with a hard timeout.
// Even on timeout/cancel it returns partial output in Text.
func TraceOnce(ctx context.Context, address string) TraceOut {
	// You asked for 60 seconds.
	const hardTimeout = 60 * time.Second

	// Faster defaults (keeps it "quick" in practice):
	// -q 1 (one probe) and -m 20 (max hops)
	// worst-case ~= 20s with -w 1, but still keep 60s hard cap for safety.
	args := []string{"-n", "-q", "1", "-w", "1", "-m", "20", address}

	tctx, cancel := context.WithTimeout(ctx, hardTimeout)
	defer cancel()

	cmd := exec.CommandContext(tctx, "/usr/sbin/traceroute", args...)

	// Bound output to avoid memory issues
	const maxOut = 256 * 1024
	var buf bytes.Buffer
	limited := &limitedWriter{w: &buf, n: maxOut}
	cmd.Stdout = limited
	cmd.Stderr = limited

	logging.Info("tracert", "%s %s", cmd.Path, strings.Join(args, " "))

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

	// heuristic: if we saw destination IP in output -> treat as OK
	// (works well with -n)
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
