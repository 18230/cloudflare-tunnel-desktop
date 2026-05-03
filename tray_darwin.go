//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework Foundation
#import <Cocoa/Cocoa.h>

@interface CloudflareTunnelTrayDelegate : NSObject
- (void)showWindow:(id)sender;
- (void)hideWindow:(id)sender;
- (void)quitApp:(id)sender;
@end

@implementation CloudflareTunnelTrayDelegate
- (void)showWindow:(id)sender {
	[NSApp unhide:nil];
	[NSApp activateIgnoringOtherApps:YES];
}

- (void)hideWindow:(id)sender {
	[NSApp hide:nil];
}

- (void)quitApp:(id)sender {
	[NSApp stop:nil];
	[NSApp abortModal];
}
@end

static NSStatusItem *cloudflareStatusItem = nil;
static CloudflareTunnelTrayDelegate *cloudflareTrayDelegate = nil;

static void cloudflareSetupTrayOnMainThread(void) {
	if (cloudflareTrayDelegate == nil) {
		cloudflareTrayDelegate = [[CloudflareTunnelTrayDelegate alloc] init];
	}
	if (cloudflareStatusItem == nil) {
		cloudflareStatusItem = [[[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength] retain];
	}
	[cloudflareStatusItem setLength:NSVariableStatusItemLength];

	NSButton *button = [cloudflareStatusItem button];
	[button setToolTip:@"Cloudflare Tunnel Desktop"];
	[button setTitle:@"CF"];
	[button setImage:nil];
	[button setImagePosition:NSNoImage];

	NSMenu *trayMenu = [[NSMenu alloc] initWithTitle:@"Cloudflare Tunnel Desktop"];
	NSMenuItem *titleItem = [[NSMenuItem alloc] initWithTitle:@"Cloudflare Tunnel Desktop" action:nil keyEquivalent:@""];
	[titleItem setEnabled:NO];
	[trayMenu addItem:titleItem];
	[trayMenu addItem:[NSMenuItem separatorItem]];

	NSMenuItem *showItem = [[NSMenuItem alloc] initWithTitle:@"显示主窗口" action:@selector(showWindow:) keyEquivalent:@""];
	[showItem setTarget:cloudflareTrayDelegate];
	[trayMenu addItem:showItem];

	NSMenuItem *hideItem = [[NSMenuItem alloc] initWithTitle:@"隐藏主窗口" action:@selector(hideWindow:) keyEquivalent:@""];
	[hideItem setTarget:cloudflareTrayDelegate];
	[trayMenu addItem:hideItem];

	[trayMenu addItem:[NSMenuItem separatorItem]];
	NSMenuItem *quitItem = [[NSMenuItem alloc] initWithTitle:@"彻底退出" action:@selector(quitApp:) keyEquivalent:@""];
	[quitItem setTarget:cloudflareTrayDelegate];
	[trayMenu addItem:quitItem];

	[cloudflareStatusItem setMenu:trayMenu];
}

@interface CloudflareTunnelTraySetup : NSObject
+ (void)setup;
@end

@implementation CloudflareTunnelTraySetup
+ (void)setup {
	cloudflareSetupTrayOnMainThread();
}
@end

static void cloudflareSetupTray(void) {
	if ([NSThread isMainThread]) {
		cloudflareSetupTrayOnMainThread();
		return;
	}
	[CloudflareTunnelTraySetup performSelectorOnMainThread:@selector(setup) withObject:nil waitUntilDone:YES];
}
*/
import "C"

// setupMenuBarTray 创建 macOS 顶部菜单栏入口，便于窗口隐藏后重新唤起或彻底退出。
func setupMenuBarTray(app *App) {
	C.cloudflareSetupTray()
}

// teardownMenuBarTray 在 macOS 退出时保留系统回收逻辑，避免重复操作状态栏对象。
func teardownMenuBarTray(app *App) {
}
