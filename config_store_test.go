package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigStorePersistsPlaintextCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewConfigStoreAt(path)
	config := NewDefaultConfig()
	config.AccountID = "0123456789abcdef0123456789abcdef"
	config.ZoneID = "abcdef0123456789abcdef0123456789"
	config.RootDomain = "example.com"
	config.TunnelID = "11111111-2222-3333-4444-555555555555"
	config.TunnelName = "desktop"
	config.APIToken = "plain-api-token"
	config.TunnelToken = "plain-tunnel-token"

	if err := store.Save(config); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	content := string(data)
	for _, expected := range []string{"plain-api-token", "plain-tunnel-token"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("config should contain %q: %s", expected, content)
		}
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.APIToken != config.APIToken || loaded.TunnelToken != config.TunnelToken {
		t.Fatalf("credentials were not loaded: %#v", loaded)
	}
}
