package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWindowsCloudflaredInstallCommandPrefersWinget 验证 Windows 自动安装优先使用官方推荐的 winget 包。
func TestWindowsCloudflaredInstallCommandPrefersWinget(t *testing.T) {
	dir := t.TempDir()
	writeFakeExecutable(t, dir, "winget")
	t.Setenv("PATH", dir)

	installer, err := cloudflaredInstallCommandForOS("windows")
	if err != nil {
		t.Fatalf("cloudflaredInstallCommandForOS returned error: %v", err)
	}
	if installer.Name != "winget" {
		t.Fatalf("expected winget installer, got %s", installer.Name)
	}
	if got := installer.Args; len(got) < 3 || got[0] != "install" || got[1] != "--id" || got[2] != "Cloudflare.cloudflared" {
		t.Fatalf("unexpected winget args: %#v", got)
	}
}

// TestWindowsCloudflaredInstallCommandFallsBackToScoop 验证未找到 winget 时可以回退到 scoop。
func TestWindowsCloudflaredInstallCommandFallsBackToScoop(t *testing.T) {
	dir := t.TempDir()
	writeFakeExecutable(t, dir, "scoop")
	t.Setenv("PATH", dir)

	installer, err := cloudflaredInstallCommandForOS("windows")
	if err != nil {
		t.Fatalf("cloudflaredInstallCommandForOS returned error: %v", err)
	}
	if installer.Name != "scoop" {
		t.Fatalf("expected scoop installer, got %s", installer.Name)
	}
}

// TestUnsupportedCloudflaredInstallCommandReturnsActionableError 验证暂未支持的平台返回可操作错误。
func TestUnsupportedCloudflaredInstallCommandReturnsActionableError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, err := cloudflaredInstallCommandForOS("linux"); err == nil {
		t.Fatal("expected unsupported platform error")
	}
}

// writeFakeExecutable 写入测试用假命令，避免依赖开发机真实包管理器。
func writeFakeExecutable(t *testing.T, dir string, name string) {
	t.Helper()
	names := []string{name}
	if runtime.GOOS == "windows" {
		names = append(names, name+".exe")
	}
	for _, currentName := range names {
		path := filepath.Join(dir, currentName)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatalf("WriteFile returned error: %v", err)
		}
	}
}
