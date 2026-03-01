//go:build !windows

package singleinstance

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var lockFile *os.File

func claimPlatform(appID string) (func() error, error) {
	// Put lock into user cache dir (better than /tmp for multi-user).
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	_ = os.MkdirAll(dir, 0o755)

	path := filepath.Join(dir, appID+".lock")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	// Non-blocking exclusive lock.
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, ErrAlreadyRunning
	}

	lockFile = f
	return func() error {
		if lockFile != nil {
			_ = unix.Flock(int(lockFile.Fd()), unix.LOCK_UN)
			_ = lockFile.Close()
			lockFile = nil
		}
		return nil
	}, nil
}
