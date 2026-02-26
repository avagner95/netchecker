//go:build windows

package monitor

import (
	"context"
	"os/exec"
	"syscall"
)

func execCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true,
		CreationFlags: syscall.CREATE_NO_WINDOW | syscall.CREATE_NEW_PROCESS_GROUP}

	return cmd
}
