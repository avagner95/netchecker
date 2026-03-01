package singleinstance

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var ErrAlreadyRunning = errors.New("already running")

func Claim(appID string) (release func() error, err error) {
	return claimPlatform(appID)
}

// ---- Focus signalling without fixed port ----

// focusFilePath returns per-user file storing chosen focus port for this appID.
func focusFilePath(appID string) string {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0o755)
	// make it unique per app
	return filepath.Join(dir, appID+".focus")
}

func writeFocusPort(appID string, port int) {
	// atomic-ish write: write temp then rename
	path := focusFilePath(appID)
	tmp := path + ".tmp"
	_ = os.WriteFile(tmp, []byte(strconv.Itoa(port)), 0o600)
	_ = os.Rename(tmp, path)
}

func readFocusPort(appID string) (int, error) {
	b, err := os.ReadFile(focusFilePath(appID))
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(b))
	p, err := strconv.Atoi(s)
	if err != nil || p <= 0 || p > 65535 {
		return 0, fmt.Errorf("bad focus port in file: %q", s)
	}
	return p, nil
}

// StartFocusServer starts a local TCP server on a random free port (127.0.0.1:0)
// and stores that port in a per-user file so secondary instances can request focus.
func StartFocusServer(appID string, onFocus func()) (stop func() error, err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0") // dynamic port
	if err != nil {
		return nil, fmt.Errorf("listen focus server: %w", err)
	}

	// store chosen port
	addr := ln.Addr().String() // "127.0.0.1:54321"
	_, portStr, _ := strings.Cut(addr, ":")
	port, _ := strconv.Atoi(portStr)
	if port > 0 {
		writeFocusPort(appID, port)
	}

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			buf := make([]byte, 16)
			n, _ := conn.Read(buf)
			_ = conn.Close()

			if n > 0 && string(buf[:n]) == "focus" {
				onFocus()
			}
		}
	}()

	return func() error {
		_ = ln.Close()
		<-done
		// best-effort cleanup
		_ = os.Remove(focusFilePath(appID))
		return nil
	}, nil
}

// RequestFocus reads the stored port and asks the primary instance to focus.
func RequestFocus(appID string) error {
	port, err := readFocusPort(appID)
	if err != nil {
		return fmt.Errorf("read focus port: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("dial focus server: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(500 * time.Millisecond))
	_, _ = conn.Write([]byte("focus"))
	_ = conn.Close()
	return nil
}
