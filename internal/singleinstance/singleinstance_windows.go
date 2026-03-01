//go:build windows

package singleinstance

import (
	"fmt"

	"golang.org/x/sys/windows"
)

var mutex windows.Handle

func claimPlatform(appID string) (func() error, error) {
	// "Global\" works across sessions; "Local\" is per-session.
	// Use Global to be safe.
	name, _ := windows.UTF16PtrFromString("Global\\" + appID)

	h, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		return nil, fmt.Errorf("CreateMutex: %w", err)
	}

	// If it already existed, then another instance is running.
	if windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		_ = windows.CloseHandle(h)
		return nil, ErrAlreadyRunning
	}

	mutex = h
	return func() error {
		if mutex != 0 {
			_ = windows.CloseHandle(mutex)
			mutex = 0
		}
		return nil
	}, nil
}
