package main

import "testing"

func TestNormalizeRouteValidatesDomainAndPort(t *testing.T) {
	route, err := NormalizeRoute(RouteInput{
		Hostname:        "App.Example.COM.",
		ServiceProtocol: "https",
		ServiceHost:     "localhost",
		ServicePort:     8443,
		Enabled:         true,
	}, "example.com")
	if err != nil {
		t.Fatalf("NormalizeRoute returned error: %v", err)
	}
	if route.Hostname != "app.example.com" {
		t.Fatalf("hostname normalized to %q", route.Hostname)
	}
	if got := BuildServiceURL(route); got != "https://localhost:8443" {
		t.Fatalf("unexpected service URL: %s", got)
	}
}

func TestNormalizeRouteRejectsOutOfZoneHostname(t *testing.T) {
	_, err := NormalizeRoute(RouteInput{
		Hostname:        "app.other.com",
		ServiceProtocol: "http",
		ServiceHost:     "localhost",
		ServicePort:     3000,
		Enabled:         true,
	}, "example.com")
	if err == nil {
		t.Fatal("expected out-of-zone hostname to be rejected")
	}
}

func TestNormalizeRouteRejectsInvalidPort(t *testing.T) {
	_, err := NormalizeRoute(RouteInput{
		Hostname:        "app.example.com",
		ServiceProtocol: "http",
		ServiceHost:     "localhost",
		ServicePort:     70000,
		Enabled:         true,
	}, "example.com")
	if err == nil {
		t.Fatal("expected invalid port to be rejected")
	}
}

func TestBuildServiceURLFormatsIPv6Host(t *testing.T) {
	route := Route{
		Hostname:        "app.example.com",
		ServiceProtocol: "http",
		ServiceHost:     "::1",
		ServicePort:     8080,
		Enabled:         true,
	}
	if got := BuildServiceURL(route); got != "http://[::1]:8080" {
		t.Fatalf("unexpected IPv6 service URL: %s", got)
	}
	if err := ValidateURLForDisplay(BuildServiceURL(route)); err != nil {
		t.Fatalf("generated URL should be valid: %v", err)
	}
}

func TestRoutesFromTunnelConfigurationImportsSupportedHTTPRoutes(t *testing.T) {
	routes, skipped := RoutesFromTunnelConfiguration(tunnelConfiguration{
		Ingress: []tunnelIngressRule{{
			Hostname: "app.example.com",
			Service:  "http://localhost:3000",
		}, {
			Hostname: "secure.example.com",
			Service:  "https://127.0.0.1",
		}, {
			Hostname: "ssh.example.com",
			Service:  "ssh://localhost:22",
		}, {
			Hostname: "path.example.com",
			Service:  "http://localhost:8080",
			Path:     "/api",
		}, {
			Service: "http_status:404",
		}},
	}, "example.com")
	if skipped != 2 {
		t.Fatalf("expected two skipped unsupported rules, got %d", skipped)
	}
	if len(routes) != 2 {
		t.Fatalf("expected two imported routes, got %#v", routes)
	}
	if routes[0].Hostname != "app.example.com" || routes[0].ServicePort != 3000 {
		t.Fatalf("unexpected first route: %#v", routes[0])
	}
	if routes[1].Hostname != "secure.example.com" || routes[1].ServicePort != 443 {
		t.Fatalf("unexpected default https route: %#v", routes[1])
	}
}

func TestRouteFromTunnelDNSRecordInfersPortAndDisablesRoute(t *testing.T) {
	route, ok := RouteFromTunnelDNSRecord(DNSRecord{
		Type:    "CNAME",
		Name:    "3000.example.com",
		Content: "11111111-2222-3333-4444-555555555555.cfargotunnel.com",
		Proxied: true,
	}, "example.com")
	if !ok {
		t.Fatal("expected DNS record to become a route")
	}
	if route.Hostname != "3000.example.com" || route.ServicePort != 3000 {
		t.Fatalf("unexpected route from DNS record: %#v", route)
	}
	if route.Enabled {
		t.Fatalf("DNS-only route should be disabled until the local service is confirmed: %#v", route)
	}
}
