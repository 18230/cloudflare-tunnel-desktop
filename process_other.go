//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// configureBackgroundCommand 在非 Windows 平台保持默认子进程行为。
func configureBackgroundCommand(cmd *exec.Cmd) {
}

// stopManagedProcess 在 Unix 平台向 cloudflared 发送 SIGTERM，让进程有机会自行清理连接。
func stopManagedProcess(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGTERM)
}
