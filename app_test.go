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

// TestApplyDiscoveredZoneUsesSingleCandidate 验证自动发现只有一个 Zone 时会回填账号和根域名。
func TestApplyDiscoveredZoneUsesSingleCandidate(t *testing.T) {
	config, messages := applyDiscoveredZone(AppConfig{}, []CloudflareZone{{
		ID:   "abcdef0123456789abcdef0123456789",
		Name: "example.com",
		Account: CloudflareAccount{
			ID:   "0123456789abcdef0123456789abcdef",
			Name: "Personal",
		},
	}}, []CloudflareAccount{{
		ID:   "0123456789abcdef0123456789abcdef",
		Name: "Personal",
	}}, nil)
	if config.AccountID != "0123456789abcdef0123456789abcdef" || config.ZoneID != "abcdef0123456789abcdef0123456789" || config.RootDomain != "example.com" {
		t.Fatalf("unexpected discovered config: %#v", config)
	}
	if len(messages) == 0 {
		t.Fatal("expected discovery messages")
	}
}

// TestApplyDiscoveredTunnelClearsForeignTunnel 验证切换账号后不会保留不属于该账号的 Tunnel。
func TestApplyDiscoveredTunnelClearsForeignTunnel(t *testing.T) {
	config, messages := applyDiscoveredTunnel(nil, nil, AppConfig{
		AccountID:   "0123456789abcdef0123456789abcdef",
		TunnelID:    "11111111-2222-3333-4444-555555555555",
		TunnelName:  "old",
		TunnelToken: "old-token",
	}, []CloudflareTunnel{{
		ID:   "22222222-3333-4444-5555-666666666666",
		Name: "new",
	}}, nil)
	if config.TunnelID != "" || config.TunnelName != "" || config.TunnelToken != "" {
		t.Fatalf("foreign tunnel should be cleared: %#v", config)
	}
	if len(messages) == 0 {
		t.Fatal("expected discovery messages")
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
		globalKey:   config.APIToken,
		tunnelToken: config.TunnelToken,
	}
}
