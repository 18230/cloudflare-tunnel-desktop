//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

const windowsCreateNoWindow = 0x08000000

// configureBackgroundCommand 隐藏 Windows 子进程控制台窗口，避免桌面应用启动或检测版本时闪黑框。
func configureBackgroundCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windowsCreateNoWindow,
	}
}

// stopManagedProcess 在 Windows 上直接结束 cloudflared 子进程，避免 SIGTERM 返回 not supported by windows。
func stopManagedProcess(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
