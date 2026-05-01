package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildCloudflaredArgsDoesNotContainToken(t *testing.T) {
	token := "eyJ.secret.token"
	args := BuildCloudflaredArgs("http2", "11111111-2222-3333-4444-555555555555")
	if strings.Contains(strings.Join(args, " "), token) {
		t.Fatal("token leaked into command args")
	}
	if got := strings.Join(args, " "); !strings.Contains(got, "--protocol http2") {
		t.Fatalf("protocol missing from args: %s", got)
	}
}

func TestTunnelManagerRunsFakeCloudflaredWithTokenInEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake is only used on unix-like systems")
	}
	dir := t.TempDir()
	fakePath := filepath.Join(dir, "cloudflared")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "cloudflared fake"
  exit 0
fi
if [ -z "$TUNNEL_TOKEN" ]; then
  echo '{"level":"error","message":"missing token"}' >&2
  exit 2
fi
echo '{"level":"info","message":"connected to edge"}'
exit 0
`
	if err := os.WriteFile(fakePath, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	original := cloudflaredExecutable
	cloudflaredExecutable = fakePath
	t.Cleanup(func() {
		cloudflaredExecutable = original
	})

	manager := NewTunnelManager(nil)
	if err := manager.Start("secret-token", "11111111-2222-3333-4444-555555555555", "auto", false); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	logs := manager.Logs()
	found := false
	for _, entry := range logs {
		if strings.Contains(entry.Message, "connected to edge") {
			found = true
		}
		if strings.Contains(entry.Message, "secret-token") {
			t.Fatal("token leaked into logs")
		}
	}
	if !found {
		t.Fatalf("expected fake cloudflared log, got %#v", logs)
	}
}
