//go:build windows

package main

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	windowsTrayID              = 1
	windowsTrayCallbackMessage = 0x8000 + 101

	windowsTrayMenuShow = 1001
	windowsTrayMenuHide = 1002
	windowsTrayMenuQuit = 1003

	wmCommand       = 0x0111
	wmClose         = 0x0010
	wmDestroy       = 0x0002
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmContextMenu   = 0x007b

	nimAdd        = 0x00000000
	nimDelete     = 0x00000002
	nimSetVersion = 0x00000004

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	notifyIconVersion4 = 4
	idiApplication     = 32512

	mfString    = 0x00000000
	mfDisabled  = 0x00000002
	mfSeparator = 0x00000800

	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100
)

var (
	windowsTrayOnce     sync.Once
	windowsTrayMu       sync.Mutex
	windowsTrayApp      *App
	windowsTrayHwnd     uintptr
	windowsTrayCallback = syscall.NewCallback(windowsTrayWndProc)

	windowsKernel32 = syscall.NewLazyDLL("kernel32.dll")
	windowsUser32   = syscall.NewLazyDLL("user32.dll")
	windowsShell32  = syscall.NewLazyDLL("shell32.dll")

	procGetModuleHandleW = windowsKernel32.NewProc("GetModuleHandleW")
	procGetModuleFileW   = windowsKernel32.NewProc("GetModuleFileNameW")
	procRegisterClassExW = windowsUser32.NewProc("RegisterClassExW")
	procCreateWindowExW  = windowsUser32.NewProc("CreateWindowExW")
	procDefWindowProcW   = windowsUser32.NewProc("DefWindowProcW")
	procDestroyWindow    = windowsUser32.NewProc("DestroyWindow")
	procPostQuitMessage  = windowsUser32.NewProc("PostQuitMessage")
	procGetMessageW      = windowsUser32.NewProc("GetMessageW")
	procTranslateMessage = windowsUser32.NewProc("TranslateMessage")
	procDispatchMessageW = windowsUser32.NewProc("DispatchMessageW")
	procPostMessageW     = windowsUser32.NewProc("PostMessageW")
	procLoadIconW        = windowsUser32.NewProc("LoadIconW")
	procGetCursorPos     = windowsUser32.NewProc("GetCursorPos")
	procSetForegroundWnd = windowsUser32.NewProc("SetForegroundWindow")
	procCreatePopupMenu  = windowsUser32.NewProc("CreatePopupMenu")
	procAppendMenuW      = windowsUser32.NewProc("AppendMenuW")
	procTrackPopupMenu   = windowsUser32.NewProc("TrackPopupMenu")
	procDestroyMenu      = windowsUser32.NewProc("DestroyMenu")
	procExtractIconW     = windowsShell32.NewProc("ExtractIconW")
	procShellNotifyIconW = windowsShell32.NewProc("Shell_NotifyIconW")
)

type windowsNotifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

type windowsWndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type windowsPoint struct {
	X int32
	Y int32
}

type windowsMsg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      windowsPoint
}

// setupMenuBarTray 创建 Windows 通知区图标，保证主窗口隐藏后仍可从托盘唤起。
func setupMenuBarTray(app *App) {
	windowsTrayMu.Lock()
	windowsTrayApp = app
	windowsTrayMu.Unlock()
	windowsTrayOnce.Do(func() {
		go runWindowsTray(app)
	})
}

// teardownMenuBarTray 删除 Windows 通知区图标，并结束托盘消息循环。
func teardownMenuBarTray(app *App) {
	windowsTrayMu.Lock()
	hwnd := windowsTrayHwnd
	windowsTrayHwnd = 0
	windowsTrayMu.Unlock()
	if hwnd == 0 {
		return
	}
	nid := newWindowsNotifyIconData(hwnd)
	procShellNotifyIconW.Call(uintptr(nimDelete), uintptr(unsafe.Pointer(&nid)))
	procPostMessageW.Call(hwnd, uintptr(wmClose), 0, 0)
}

// runWindowsTray 在独立 UI 线程中注册隐藏窗口并运行托盘消息循环。
func runWindowsTray(app *App) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	className, _ := syscall.UTF16PtrFromString("CloudflareTunnelDesktopTray")
	hInstance, _, _ := procGetModuleHandleW.Call(0)
	wndClass := windowsWndClassEx{
		CbSize:        uint32(unsafe.Sizeof(windowsWndClassEx{})),
		LpfnWndProc:   windowsTrayCallback,
		HInstance:     hInstance,
		LpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wndClass)))
	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		0,
		0,
		0,
		0,
		0,
		0,
		0,
		hInstance,
		0,
	)
	if hwnd == 0 {
		if app != nil && app.manager != nil {
			app.manager.addLog("error", "app", "Windows 托盘窗口创建失败", "tray")
		}
		return
	}

	windowsTrayMu.Lock()
	windowsTrayHwnd = hwnd
	windowsTrayMu.Unlock()
	if !addWindowsTrayIcon(hwnd) && app != nil && app.manager != nil {
		app.manager.addLog("error", "app", "Windows 通知区图标创建失败", "tray")
	}

	var msg windowsMsg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// addWindowsTrayIcon 向 Windows 通知区添加应用图标和回调消息。
func addWindowsTrayIcon(hwnd uintptr) bool {
	nid := newWindowsNotifyIconData(hwnd)
	if ret, _, _ := procShellNotifyIconW.Call(uintptr(nimAdd), uintptr(unsafe.Pointer(&nid))); ret == 0 {
		return false
	}
	nid.UVersion = notifyIconVersion4
	ret, _, _ := procShellNotifyIconW.Call(uintptr(nimSetVersion), uintptr(unsafe.Pointer(&nid)))
	return ret != 0
}

// newWindowsNotifyIconData 生成 Shell_NotifyIcon 使用的固定结构体。
func newWindowsNotifyIconData(hwnd uintptr) windowsNotifyIconData {
	nid := windowsNotifyIconData{
		CbSize:           uint32(unsafe.Sizeof(windowsNotifyIconData{})),
		HWnd:             hwnd,
		UID:              windowsTrayID,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: windowsTrayCallbackMessage,
		HIcon:            loadWindowsTrayIcon(),
	}
	tip, _ := syscall.UTF16FromString("Cloudflare Tunnel Desktop")
	copy(nid.SzTip[:], tip)
	return nid
}

// loadWindowsTrayIcon 从当前 exe 提取应用图标，使托盘图标和 Windows 应用图标保持一致。
func loadWindowsTrayIcon() uintptr {
	var exePath [260]uint16
	length, _, _ := procGetModuleFileW.Call(0, uintptr(unsafe.Pointer(&exePath[0])), uintptr(len(exePath)))
	if length > 0 {
		if hIcon, _, _ := procExtractIconW.Call(0, uintptr(unsafe.Pointer(&exePath[0])), 0); hIcon > 1 {
			return hIcon
		}
	}
	hIcon, _, _ := procLoadIconW.Call(0, uintptr(idiApplication))
	return hIcon
}

// windowsTrayWndProc 处理托盘点击和菜单命令。
func windowsTrayWndProc(hwnd uintptr, msg uint32, wparam uintptr, lparam uintptr) uintptr {
	switch msg {
	case windowsTrayCallbackMessage:
		switch uint32(lparam) {
		case wmLButtonUp, wmLButtonDblClk:
			showWindowsTrayMenu(hwnd)
			return 0
		case wmRButtonUp, wmContextMenu:
			showWindowsTrayMenu(hwnd)
			return 0
		}
	case wmCommand:
		handleWindowsTrayCommand(uint32(wparam & 0xffff))
		return 0
	case wmClose:
		nid := newWindowsNotifyIconData(hwnd)
		procShellNotifyIconW.Call(uintptr(nimDelete), uintptr(unsafe.Pointer(&nid)))
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return ret
}

// showWindowsTrayMenu 在鼠标当前位置显示 Windows 托盘菜单。
func showWindowsTrayMenu(hwnd uintptr) {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	appendWindowsTrayMenu(menu, mfString|mfDisabled, 0, "Cloudflare Tunnel Desktop")
	appendWindowsTrayMenu(menu, mfSeparator, 0, "")
	appendWindowsTrayMenu(menu, mfString, windowsTrayMenuShow, "显示主窗口")
	appendWindowsTrayMenu(menu, mfString, windowsTrayMenuHide, "隐藏主窗口")
	appendWindowsTrayMenu(menu, mfSeparator, 0, "")
	appendWindowsTrayMenu(menu, mfString, windowsTrayMenuQuit, "彻底退出")

	var point windowsPoint
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	procSetForegroundWnd.Call(hwnd)
	command, _, _ := procTrackPopupMenu.Call(
		menu,
		uintptr(tpmRightButton|tpmReturnCmd),
		uintptr(point.X),
		uintptr(point.Y),
		0,
		hwnd,
		0,
	)
	procDestroyMenu.Call(menu)
	if command != 0 {
		handleWindowsTrayCommand(uint32(command))
	}
}

// appendWindowsTrayMenu 追加 Windows 托盘菜单项。
func appendWindowsTrayMenu(menu uintptr, flags uint32, id uintptr, label string) {
	var labelPtr *uint16
	if label != "" {
		labelPtr, _ = syscall.UTF16PtrFromString(label)
	}
	procAppendMenuW.Call(menu, uintptr(flags), id, uintptr(unsafe.Pointer(labelPtr)))
}

// handleWindowsTrayCommand 执行托盘菜单命令。
func handleWindowsTrayCommand(command uint32) {
	switch command {
	case windowsTrayMenuShow:
		windowsTrayShowMainWindow()
	case windowsTrayMenuHide:
		windowsTrayWithApp(func(app *App) {
			app.HideMainWindow()
		})
	case windowsTrayMenuQuit:
		windowsTrayWithApp(func(app *App) {
			app.QuitApplication()
		})
	}
}

// windowsTrayShowMainWindow 显示并聚焦主窗口。
func windowsTrayShowMainWindow() {
	windowsTrayWithApp(func(app *App) {
		app.ShowMainWindow()
	})
}

// windowsTrayWithApp 安全读取当前应用实例，避免托盘线程和 Wails 生命周期竞争。
func windowsTrayWithApp(fn func(*App)) {
	windowsTrayMu.Lock()
	app := windowsTrayApp
	windowsTrayMu.Unlock()
	if app == nil {
		return
	}
	fn(app)
}
