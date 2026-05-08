package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCloudflareClientCreatesTunnelWithGlobalKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/0123456789abcdef0123456789abcdef/cfd_tunnel" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("global key request should not use bearer auth: %s", got)
		}
		if got := r.Header.Get("X-Auth-Email"); got != "owner@example.com" {
			t.Fatalf("unexpected auth email: %s", got)
		}
		if got := r.Header.Get("X-Auth-Key"); got != "global-key" {
			t.Fatalf("unexpected auth key: %s", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if body["config_src"] != "cloudflare" {
			t.Fatalf("unexpected config_src: %#v", body)
		}
		writeCloudflareResult(w, CloudflareTunnel{
			ID:    "11111111-2222-3333-4444-555555555555",
			Name:  "desktop",
			Token: "tunnel-token",
		})
	}))
	defer server.Close()

	client := NewCloudflareClientWithBaseURL(server.URL, "owner@example.com", "global-key")
	tunnel, err := client.CreateTunnel(context.Background(), "0123456789abcdef0123456789abcdef", "desktop")
	if err != nil {
		t.Fatalf("CreateTunnel returned error: %v", err)
	}
	if tunnel.ID != "11111111-2222-3333-4444-555555555555" || tunnel.Token != "tunnel-token" {
		t.Fatalf("unexpected tunnel: %#v", tunnel)
	}
}

func TestCloudflareClientUsesGlobalAPIKeyHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("global key request should not use bearer auth: %s", got)
		}
		if got := r.Header.Get("X-Auth-Email"); got != "owner@example.com" {
			t.Fatalf("unexpected auth email: %s", got)
		}
		if got := r.Header.Get("X-Auth-Key"); got != "global-key" {
			t.Fatalf("unexpected auth key: %s", got)
		}
		writeCloudflareResult(w, CloudflareZone{
			ID:   "abcdef0123456789abcdef0123456789",
			Name: "example.com",
		})
	}))
	defer server.Close()

	client := NewCloudflareClientWithBaseURLAndAuth(server.URL, CloudflareAuth{
		Type:  authTypeGlobalKey,
		Email: "owner@example.com",
		Key:   "global-key",
	})
	zone, err := client.GetZone(context.Background(), "abcdef0123456789abcdef0123456789")
	if err != nil {
		t.Fatalf("GetZone returned error: %v", err)
	}
	if zone.Name != "example.com" {
		t.Fatalf("unexpected zone: %#v", zone)
	}
}

func TestCloudflareClientPutTunnelConfigurationAddsCatchAll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/0123456789abcdef0123456789abcdef/cfd_tunnel/11111111-2222-3333-4444-555555555555/configurations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body tunnelConfigurationPayload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode returned error: %v", err)
		}
		if len(body.Config.Ingress) != 2 {
			t.Fatalf("expected route plus catch-all, got %#v", body.Config.Ingress)
		}
		if body.Config.Ingress[0].Hostname != "app.example.com" {
			t.Fatalf("unexpected hostname: %#v", body.Config.Ingress[0])
		}
		if body.Config.Ingress[0].Service != "http://localhost:3000" {
			t.Fatalf("unexpected service: %#v", body.Config.Ingress[0])
		}
		if body.Config.Ingress[1].Service != "http_status:404" {
			t.Fatalf("missing catch-all: %#v", body.Config.Ingress[1])
		}
		writeCloudflareResult(w, map[string]any{"ok": true})
	}))
	defer server.Close()

	client := NewCloudflareClientWithBaseURL(server.URL, "owner@example.com", "global-key")
	err := client.PutTunnelConfiguration(context.Background(), "0123456789abcdef0123456789abcdef", "11111111-2222-3333-4444-555555555555", []Route{{
		Hostname:        "app.example.com",
		ServiceProtocol: "http",
		ServiceHost:     "localhost",
		ServicePort:     3000,
		Enabled:         true,
	}})
	if err != nil {
		t.Fatalf("PutTunnelConfiguration returned error: %v", err)
	}
}

func TestCloudflareClientGetTunnelConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/0123456789abcdef0123456789abcdef/cfd_tunnel/11111111-2222-3333-4444-555555555555/configurations" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		writeCloudflareResult(w, tunnelConfigurationResponse{
			Config: tunnelConfiguration{
				Ingress: []tunnelIngressRule{{
					Hostname: "app.example.com",
					Service:  "https://localhost:8443",
				}, {
					Service: "http_status:404",
				}},
			},
		})
	}))
	defer server.Close()

	client := NewCloudflareClientWithBaseURL(server.URL, "owner@example.com", "global-key")
	config, err := client.GetTunnelConfiguration(context.Background(), "0123456789abcdef0123456789abcdef", "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatalf("GetTunnelConfiguration returned error: %v", err)
	}
	if len(config.Ingress) != 2 || config.Ingress[0].Hostname != "app.example.com" {
		t.Fatalf("unexpected tunnel configuration: %#v", config)
	}
}

func TestCloudflareClientEnsureDNSRecordCreatesCNAME(t *testing.T) {
	var sawCreate bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/zones/abcdef0123456789abcdef0123456789/dns_records":
			if r.URL.Query().Get("type") != "CNAME" || r.URL.Query().Get("name") != "app.example.com" {
				t.Fatalf("unexpected query: %s", r.URL.RawQuery)
			}
			writeCloudflareResult(w, []DNSRecord{})
		case r.Method == http.MethodPost && r.URL.Path == "/zones/abcdef0123456789abcdef0123456789/dns_records":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("Decode returned error: %v", err)
			}
			if body["type"] != "CNAME" || body["name"] != "app.example.com" {
				t.Fatalf("unexpected DNS body: %#v", body)
			}
			content, _ := body["content"].(string)
			if !strings.HasSuffix(content, ".cfargotunnel.com") {
				t.Fatalf("unexpected DNS content: %s", content)
			}
			if body["proxied"] != true {
				t.Fatalf("expected proxied=true: %#v", body)
			}
			sawCreate = true
			writeCloudflareResult(w, DNSRecord{ID: "dns-1", Name: "app.example.com", Type: "CNAME", Content: content, Proxied: true})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewCloudflareClientWithBaseURL(server.URL, "owner@example.com", "global-key")
	record, err := client.EnsureTunnelDNSRecord(context.Background(), "abcdef0123456789abcdef0123456789", "11111111-2222-3333-4444-555555555555", "app.example.com")
	if err != nil {
		t.Fatalf("EnsureTunnelDNSRecord returned error: %v", err)
	}
	if !sawCreate || record.ID != "dns-1" {
		t.Fatalf("DNS record was not created: %#v", record)
	}
}

func TestCloudflareClientListTunnelDNSRecordsFiltersTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/zones/abcdef0123456789abcdef0123456789/dns_records" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("type") != "CNAME" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		writeCloudflareResult(w, []DNSRecord{{
			ID:      "dns-1",
			Type:    "CNAME",
			Name:    "3000.example.com",
			Content: "11111111-2222-3333-4444-555555555555.cfargotunnel.com",
			Proxied: true,
		}, {
			ID:      "dns-2",
			Type:    "CNAME",
			Name:    "other.example.com",
			Content: "22222222-2222-3333-4444-555555555555.cfargotunnel.com",
			Proxied: true,
		}})
	}))
	defer server.Close()

	client := NewCloudflareClientWithBaseURL(server.URL, "owner@example.com", "global-key")
	records, err := client.ListTunnelDNSRecords(context.Background(), "abcdef0123456789abcdef0123456789", "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatalf("ListTunnelDNSRecords returned error: %v", err)
	}
	if len(records) != 1 || records[0].Name != "3000.example.com" {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestCloudflareClientDeleteTunnelDNSRecordsDeletesOnlyTargetRecords(t *testing.T) {
	deletedIDs := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/zones/abcdef0123456789abcdef0123456789/dns_records":
			writeCloudflareResult(w, []DNSRecord{{
				ID:      "dns-1",
				Type:    "CNAME",
				Name:    "3000.example.com",
				Content: "11111111-2222-3333-4444-555555555555.cfargotunnel.com",
				Proxied: true,
			}, {
				ID:      "dns-2",
				Type:    "CNAME",
				Name:    "other.example.com",
				Content: "22222222-2222-3333-4444-555555555555.cfargotunnel.com",
				Proxied: true,
			}})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/zones/abcdef0123456789abcdef0123456789/dns_records/"):
			deletedIDs = append(deletedIDs, strings.TrimPrefix(r.URL.Path, "/zones/abcdef0123456789abcdef0123456789/dns_records/"))
			writeCloudflareResult(w, map[string]any{"id": deletedIDs[len(deletedIDs)-1]})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewCloudflareClientWithBaseURL(server.URL, "owner@example.com", "global-key")
	deleted, err := client.DeleteTunnelDNSRecords(context.Background(), "abcdef0123456789abcdef0123456789", "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatalf("DeleteTunnelDNSRecords returned error: %v", err)
	}
	if deleted != 1 || len(deletedIDs) != 1 || deletedIDs[0] != "dns-1" {
		t.Fatalf("unexpected deleted DNS records: count=%d ids=%#v", deleted, deletedIDs)
	}
}

func TestCloudflareClientGetZone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones/abcdef0123456789abcdef0123456789" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeCloudflareResult(w, CloudflareZone{
			ID:     "abcdef0123456789abcdef0123456789",
			Name:   "example.com",
			Status: "active",
			Account: CloudflareAccount{
				ID:   "0123456789abcdef0123456789abcdef",
				Name: "main",
			},
		})
	}))
	defer server.Close()

	client := NewCloudflareClientWithBaseURL(server.URL, "owner@example.com", "global-key")
	zone, err := client.GetZone(context.Background(), "abcdef0123456789abcdef0123456789")
	if err != nil {
		t.Fatalf("GetZone returned error: %v", err)
	}
	if zone.Name != "example.com" || zone.Account.ID != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected zone: %#v", zone)
	}
}

func TestCloudflareClientListAndDeleteTunnels(t *testing.T) {
	var sawDelete bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/accounts/0123456789abcdef0123456789abcdef/cfd_tunnel":
			if r.URL.Query().Get("per_page") != "50" {
				t.Fatalf("missing pagination query: %s", r.URL.RawQuery)
			}
			writeCloudflareResult(w, []CloudflareTunnel{{
				ID:     "11111111-2222-3333-4444-555555555555",
				Name:   "desktop",
				Status: "healthy",
			}})
		case r.Method == http.MethodDelete && r.URL.Path == "/accounts/0123456789abcdef0123456789abcdef/cfd_tunnel/11111111-2222-3333-4444-555555555555":
			sawDelete = true
			writeCloudflareResult(w, map[string]any{"id": "11111111-2222-3333-4444-555555555555"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewCloudflareClientWithBaseURL(server.URL, "owner@example.com", "global-key")
	tunnels, err := client.ListTunnels(context.Background(), "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("ListTunnels returned error: %v", err)
	}
	if len(tunnels) != 1 || tunnels[0].Name != "desktop" {
		t.Fatalf("unexpected tunnels: %#v", tunnels)
	}
	if err := client.DeleteTunnel(context.Background(), "0123456789abcdef0123456789abcdef", "11111111-2222-3333-4444-555555555555"); err != nil {
		t.Fatalf("DeleteTunnel returned error: %v", err)
	}
	if !sawDelete {
		t.Fatal("delete request was not observed")
	}
}

func TestCloudflareClientListZonesFiltersByAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/zones" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("account.id") != "0123456789abcdef0123456789abcdef" {
			t.Fatalf("missing account filter: %s", r.URL.RawQuery)
		}
		writeCloudflareResult(w, []CloudflareZone{{
			ID:     "abcdef0123456789abcdef0123456789",
			Name:   "example.com",
			Status: "active",
		}})
	}))
	defer server.Close()

	client := NewCloudflareClientWithBaseURL(server.URL, "owner@example.com", "global-key")
	zones, err := client.ListZones(context.Background(), "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("ListZones returned error: %v", err)
	}
	if len(zones) != 1 || zones[0].Name != "example.com" {
		t.Fatalf("unexpected zones: %#v", zones)
	}
}

func TestCloudflareClientListAccounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("per_page") != "50" {
			t.Fatalf("missing pagination query: %s", r.URL.RawQuery)
		}
		writeCloudflareResult(w, []CloudflareAccount{{
			ID:   "0123456789abcdef0123456789abcdef",
			Name: "Personal",
		}})
	}))
	defer server.Close()

	client := NewCloudflareClientWithBaseURLAndAuth(server.URL, CloudflareAuth{
		Type:  authTypeGlobalKey,
		Email: "owner@example.com",
		Key:   "global-key",
	})
	accounts, err := client.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListAccounts returned error: %v", err)
	}
	if len(accounts) != 1 || accounts[0].Name != "Personal" {
		t.Fatalf("unexpected accounts: %#v", accounts)
	}
}

func writeCloudflareResult(w http.ResponseWriter, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":  true,
		"errors":   []any{},
		"messages": []any{},
		"result":   result,
	})
}
