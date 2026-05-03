//go:build !windows

package main

import "os/exec"

// configureBackgroundCommand 在非 Windows 平台保持默认子进程行为。
func configureBackgroundCommand(cmd *exec.Cmd) {
}
