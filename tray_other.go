//go:build !darwin && !windows

package main

// setupMenuBarTray 在暂未实现原生托盘的平台保持空实现。
func setupMenuBarTray(app *App) {
}

// teardownMenuBarTray 在暂未实现原生托盘的平台保持空实现。
func teardownMenuBarTray(app *App) {
}
