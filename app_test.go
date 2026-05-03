package main

import (
	"path/filepath"
	"testing"
)

// TestBindTunnelClearsStaleTunnelToken 验证切换到无 token 的 Tunnel 时不会沿用旧运行凭据。
func TestBindTunnelClearsStaleTunnelToken(t *testing.T) {
	app := newTestAppWithConfig(t, AppConfig{
		TunnelID:    "11111111-2222-3333-4444-555555555555",
		TunnelName:  "old",
		TunnelToken: "old-token",
	})
	app.tunnelToken = "old-token"

	config, err := app.BindTunnel(CloudflareTunnel{
		ID:   "22222222-3333-4444-5555-666666666666",
		Name: "new",
	})
	if err != nil {
		t.Fatalf("BindTunnel returned error: %v", err)
	}
	if config.TunnelToken != "" || app.tunnelToken != "" {
		t.Fatalf("stale tunnel token should be cleared: config=%q app=%q", config.TunnelToken, app.tunnelToken)
	}
	loaded, err := app.store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.TunnelToken != "" {
		t.Fatalf("saved config should not keep stale tunnel token: %q", loaded.TunnelToken)
	}
}

// TestBindTunnelPersistsReturnedTunnelToken 验证 Cloudflare 返回 token 时会保存为当前 Tunnel 凭据。
func TestBindTunnelPersistsReturnedTunnelToken(t *testing.T) {
	app := newTestAppWithConfig(t, AppConfig{
		TunnelID:    "11111111-2222-3333-4444-555555555555",
		TunnelName:  "old",
		TunnelToken: "old-token",
	})
	app.tunnelToken = "old-token"

	config, err := app.BindTunnel(CloudflareTunnel{
		ID:    "22222222-3333-4444-5555-666666666666",
		Name:  "new",
		Token: "new-token",
	})
	if err != nil {
		t.Fatalf("BindTunnel returned error: %v", err)
	}
	if config.TunnelToken != "new-token" || app.tunnelToken != "new-token" {
		t.Fatalf("new tunnel token was not saved: config=%q app=%q", config.TunnelToken, app.tunnelToken)
	}
}

// newTestAppWithConfig 创建带临时配置文件的 App，避免测试读写真实用户配置。
func newTestAppWithConfig(t *testing.T, config AppConfig) *App {
	t.Helper()
	if config.Protocol == "" {
		config.Protocol = defaultProtocol
	}
	if config.AuthType == "" {
		config.AuthType = defaultAuthType
	}
	if config.Routes == nil {
		config.Routes = []Route{}
	}
	store := NewConfigStoreAt(filepath.Join(t.TempDir(), "config.json"))
	if err := store.Save(config); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	return &App{
		store:       store,
		manager:     NewTunnelManager(nil),
		config:      config,
		authType:    NormalizeAuthType(config.AuthType),
		authEmail:   config.AuthEmail,
		apiToken:    config.APIToken,
		tunnelToken: config.TunnelToken,
	}
}
