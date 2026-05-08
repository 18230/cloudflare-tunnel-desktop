package main

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	cloudflareIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
	uuidPattern         = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
	labelPattern        = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
)

// NormalizeSettings 清理并校验基础设置，返回可落盘的配置片段。
func NormalizeSettings(input SettingsInput) (SettingsInput, error) {
	input.AccountID = strings.TrimSpace(input.AccountID)
	input.ZoneID = strings.TrimSpace(input.ZoneID)
	input.RootDomain = normalizeHostname(input.RootDomain)
	input.TunnelID = strings.TrimSpace(input.TunnelID)
	input.TunnelName = strings.TrimSpace(input.TunnelName)
	input.Protocol = normalizeProtocol(input.Protocol)

	if input.AccountID != "" && !cloudflareIDPattern.MatchString(input.AccountID) {
		return input, fmt.Errorf("Account ID 必须是 32 位十六进制字符串")
	}
	if input.ZoneID != "" && !cloudflareIDPattern.MatchString(input.ZoneID) {
		return input, fmt.Errorf("Zone ID 必须是 32 位十六进制字符串")
	}
	if input.RootDomain != "" && !isValidHostname(input.RootDomain) {
		return input, fmt.Errorf("根域名格式不正确")
	}
	if input.TunnelID != "" && !uuidPattern.MatchString(input.TunnelID) {
		return input, fmt.Errorf("Tunnel ID 必须是 UUID 格式")
	}
	if input.TunnelName != "" && len(input.TunnelName) > 120 {
		return input, fmt.Errorf("Tunnel 名称不能超过 120 个字符")
	}
	if !isValidProtocol(input.Protocol) {
		return input, fmt.Errorf("传输协议只支持 auto、quic 或 http2")
	}
	return input, nil
}

// NormalizeAuthType 统一认证方式；当前只支持 Global API Key。
func NormalizeAuthType(value string) string {
	return authTypeGlobalKey
}

// NormalizeRoute 清理并校验路由配置，保证生成的服务地址安全可控。
func NormalizeRoute(input RouteInput, rootDomain string) (Route, error) {
	route := Route{
		ID:              strings.TrimSpace(input.ID),
		Hostname:        normalizeHostname(input.Hostname),
		ServiceProtocol: strings.ToLower(strings.TrimSpace(input.ServiceProtocol)),
		ServiceHost:     strings.TrimSpace(input.ServiceHost),
		ServicePort:     input.ServicePort,
		Enabled:         input.Enabled,
	}
	if route.ServiceProtocol == "" {
		route.ServiceProtocol = "http"
	}
	if route.ServiceHost == "" {
		route.ServiceHost = defaultServiceHost
	}
	if !isValidHostname(route.Hostname) {
		return route, fmt.Errorf("公开域名格式不正确")
	}
	rootDomain = normalizeHostname(rootDomain)
	if rootDomain != "" && route.Hostname != rootDomain && !strings.HasSuffix(route.Hostname, "."+rootDomain) {
		return route, fmt.Errorf("公开域名必须属于根域名 %s", rootDomain)
	}
	if route.ServiceProtocol != "http" && route.ServiceProtocol != "https" {
		return route, fmt.Errorf("本地服务协议只支持 http 或 https")
	}
	if !isValidServiceHost(route.ServiceHost) {
		return route, fmt.Errorf("本地服务主机格式不正确")
	}
	if route.ServicePort < 1 || route.ServicePort > 65535 {
		return route, fmt.Errorf("本地端口必须在 1 到 65535 之间")
	}
	return route, nil
}

// RoutesFromTunnelConfiguration 将 Cloudflare ingress 配置转换为本应用可编辑的 HTTP/HTTPS 映射。
func RoutesFromTunnelConfiguration(config tunnelConfiguration, rootDomain string) ([]Route, int) {
	routes := []Route{}
	skipped := 0
	seenHostnames := map[string]struct{}{}
	for _, rule := range config.Ingress {
		if isTunnelCatchAllRule(rule) {
			continue
		}
		route, ok := RouteFromIngressRule(rule, rootDomain)
		if !ok {
			skipped++
			continue
		}
		if _, exists := seenHostnames[route.Hostname]; exists {
			skipped++
			continue
		}
		seenHostnames[route.Hostname] = struct{}{}
		routes = append(routes, route)
	}
	return routes, skipped
}

// RouteFromTunnelDNSRecord 将仅存在于 DNS 层的 Tunnel 记录转成待确认映射。
func RouteFromTunnelDNSRecord(record DNSRecord, rootDomain string) (Route, bool) {
	port := inferPortFromHostname(record.Name, rootDomain)
	route, err := NormalizeRoute(RouteInput{
		Hostname:        record.Name,
		ServiceProtocol: "http",
		ServiceHost:     defaultServiceHost,
		ServicePort:     port,
		Enabled:         false,
	}, rootDomain)
	if err != nil {
		return Route{}, false
	}
	return route, true
}

// RouteFromIngressRule 解析单条 ingress 规则；首版只接收无 path 的 HTTP/HTTPS 本地服务。
func RouteFromIngressRule(rule tunnelIngressRule, rootDomain string) (Route, bool) {
	if strings.TrimSpace(rule.Hostname) == "" || strings.TrimSpace(rule.Path) != "" {
		return Route{}, false
	}
	service := strings.TrimSpace(rule.Service)
	parsed, err := url.Parse(service)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return Route{}, false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return Route{}, false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return Route{}, false
	}
	portText := parsed.Port()
	if portText == "" {
		if scheme == "https" {
			portText = "443"
		} else {
			portText = "80"
		}
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return Route{}, false
	}
	route, err := NormalizeRoute(RouteInput{
		Hostname:        rule.Hostname,
		ServiceProtocol: scheme,
		ServiceHost:     parsed.Hostname(),
		ServicePort:     port,
		Enabled:         true,
	}, rootDomain)
	if err != nil {
		return Route{}, false
	}
	return route, true
}

// isTunnelCatchAllRule 识别 Cloudflare ingress 要求的兜底规则，导入时不作为用户映射展示。
func isTunnelCatchAllRule(rule tunnelIngressRule) bool {
	return strings.TrimSpace(rule.Hostname) == "" && strings.HasPrefix(strings.TrimSpace(rule.Service), "http_status:")
}

// inferPortFromHostname 从 3000.example.com 这类域名推断本地端口，无法推断时回落到 3000。
func inferPortFromHostname(hostname string, rootDomain string) int {
	hostname = normalizeHostname(hostname)
	rootDomain = normalizeHostname(rootDomain)
	prefix := hostname
	if rootDomain != "" && hostname != rootDomain && strings.HasSuffix(hostname, "."+rootDomain) {
		prefix = strings.TrimSuffix(hostname, "."+rootDomain)
	}
	label := strings.Split(prefix, ".")[0]
	port, err := strconv.Atoi(label)
	if err == nil && port >= 1 && port <= 65535 {
		return port
	}
	return 3000
}

// BuildServiceURL 根据路由生成 cloudflared ingress 需要的 service URL。
func BuildServiceURL(route Route) string {
	host := route.ServiceHost
	if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("%s://%s:%d", route.ServiceProtocol, host, route.ServicePort)
}

// ValidateConfigForSync 校验执行 Cloudflare 同步所需的配置。
func ValidateConfigForSync(config AppConfig) error {
	if !cloudflareIDPattern.MatchString(config.AccountID) {
		return fmt.Errorf("请先配置有效的 Account ID")
	}
	if !cloudflareIDPattern.MatchString(config.ZoneID) {
		return fmt.Errorf("请先配置有效的 Zone ID")
	}
	if !uuidPattern.MatchString(config.TunnelID) {
		return fmt.Errorf("请先创建或填写有效的 Tunnel ID")
	}
	if !isValidHostname(config.RootDomain) {
		return fmt.Errorf("请先配置有效的根域名")
	}
	for _, route := range config.Routes {
		if _, err := NormalizeRoute(RouteInput(route), config.RootDomain); err != nil {
			return fmt.Errorf("%s: %w", route.Hostname, err)
		}
	}
	return nil
}

// ValidateConfigForRun 校验启动 cloudflared 所需的配置。
func ValidateConfigForRun(config AppConfig) error {
	if !uuidPattern.MatchString(config.TunnelID) {
		return fmt.Errorf("请先创建或填写有效的 Tunnel ID")
	}
	if !isValidProtocol(config.Protocol) {
		return fmt.Errorf("传输协议只支持 auto、quic 或 http2")
	}
	return nil
}

// normalizeHostname 统一域名大小写并去除末尾点号。
func normalizeHostname(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	return strings.TrimSuffix(value, ".")
}

// normalizeProtocol 在空值时回落到默认传输协议。
func normalizeProtocol(protocol string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		return defaultProtocol
	}
	return protocol
}

// isValidProtocol 判断 cloudflared 传输协议是否在首版支持范围内。
func isValidProtocol(protocol string) bool {
	switch normalizeProtocol(protocol) {
	case "auto", "quic", "http2":
		return true
	default:
		return false
	}
}

// isValidHostname 校验普通域名，首版不支持通配符和空标签。
func isValidHostname(hostname string) bool {
	hostname = normalizeHostname(hostname)
	if hostname == "" || len(hostname) > 253 || strings.Contains(hostname, "*") {
		return false
	}
	labels := strings.Split(hostname, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if !labelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

// isValidServiceHost 校验本地服务主机，禁止携带 scheme、路径和端口。
func isValidServiceHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || len(host) > 253 || strings.Contains(host, "/") || strings.Contains(host, "@") {
		return false
	}
	if strings.Contains(host, "://") {
		return false
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	}
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	if strings.Contains(host, ":") {
		return false
	}
	if host == "localhost" {
		return true
	}
	labels := strings.Split(strings.ToLower(host), ".")
	for _, label := range labels {
		if !labelPattern.MatchString(label) {
			return false
		}
	}
	return true
}

// ValidateURLForDisplay 解析生成的本地服务 URL，测试中用于防止拼接出错。
func ValidateURLForDisplay(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("不支持的 URL 协议")
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("URL 缺少主机")
	}
	port := parsed.Port()
	if port == "" {
		return fmt.Errorf("URL 缺少端口")
	}
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return fmt.Errorf("URL 端口不合法")
	}
	return nil
}
