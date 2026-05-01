//go:build !darwin

package main

// setupMenuBarTray 在非 macOS 平台保持空实现，避免影响后续 Windows 扩展。
func setupMenuBarTray(app *App) {
}
