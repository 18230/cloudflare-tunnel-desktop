package main

import "time"

const (
	defaultProtocol    = "auto"
	defaultServiceHost = "localhost"
	defaultAuthType    = "global_key"
	authTypeGlobalKey  = "global_key"
	maxLogEntries      = 500
)

// AppConfig 保存本地配置；为兼容旧版本，Global API Key 仍沿用 apiToken 字段落盘。
type AppConfig struct {
	AccountID   string  `json:"accountId"`
	ZoneID      string  `json:"zoneId"`
	RootDomain  string  `json:"rootDomain"`
	TunnelID    string  `json:"tunnelId"`
	TunnelName  string  `json:"tunnelName"`
	Protocol    string  `json:"protocol"`
	AutoRestart bool    `json:"autoRestart"`
	AuthType    string  `json:"authType"`
	AuthEmail   string  `json:"authEmail"`
	APIToken    string  `json:"apiToken"`
	TunnelToken string  `json:"tunnelToken"`
	Routes      []Route `json:"routes"`
}

// Route 描述一个公开域名到本地 HTTP/HTTPS 服务的映射。
type Route struct {
	ID              string `json:"id"`
	Hostname        string `json:"hostname"`
	ServiceProtocol string `json:"serviceProtocol"`
	ServiceHost     string `json:"serviceHost"`
	ServicePort     int    `json:"servicePort"`
	Enabled         bool   `json:"enabled"`
}

// SettingsInput 接收前端提交的基础 Cloudflare 配置。
type SettingsInput struct {
	AccountID   string `json:"accountId"`
	ZoneID      string `json:"zoneId"`
	RootDomain  string `json:"rootDomain"`
	TunnelID    string `json:"tunnelId"`
	TunnelName  string `json:"tunnelName"`
	Protocol    string `json:"protocol"`
	AutoRestart bool   `json:"autoRestart"`
}

// CredentialsInput 接收需要保存到本地配置文件的 Cloudflare Global API Key 凭据。
type CredentialsInput struct {
	AuthType    string `json:"authType"`
	AuthEmail   string `json:"authEmail"`
	APIToken    string `json:"apiToken"`
	TunnelToken string `json:"tunnelToken"`
}

// RouteInput 接收前端新增或编辑路由时的表单值。
type RouteInput struct {
	ID              string `json:"id"`
	Hostname        string `json:"hostname"`
	ServiceProtocol string `json:"serviceProtocol"`
	ServiceHost     string `json:"serviceHost"`
	ServicePort     int    `json:"servicePort"`
	Enabled         bool   `json:"enabled"`
}

// RuntimeStatus 汇总当前应用会话和 cloudflared 运行状态。
type RuntimeStatus struct {
	Configured         bool   `json:"configured"`
	AuthType           string `json:"authType"`
	APITokenSet        bool   `json:"apiTokenSet"`
	TunnelTokenSet     bool   `json:"tunnelTokenSet"`
	CloudflaredPath    string `json:"cloudflaredPath"`
	CloudflaredVersion string `json:"cloudflaredVersion"`
	Running            bool   `json:"running"`
	PID                int    `json:"pid"`
	Protocol           string `json:"protocol"`
	TunnelStatus       string `json:"tunnelStatus"`
	UptimeSeconds      int64  `json:"uptimeSeconds"`
	LastError          string `json:"lastError"`
	LastEvent          string `json:"lastEvent"`
	AutoRestart        bool   `json:"autoRestart"`
	RestartAttempts    int    `json:"restartAttempts"`
	RouteCount         int    `json:"routeCount"`
	HasTunnelID        bool   `json:"hasTunnelId"`
}

// CloudflaredInstallStatus 表示 cloudflared 本地安装和安装过程状态。
type CloudflaredInstallStatus struct {
	Installed  bool     `json:"installed"`
	Installing bool     `json:"installing"`
	Path       string   `json:"path"`
	Version    string   `json:"version"`
	Status     string   `json:"status"`
	Error      string   `json:"error"`
	Logs       []string `json:"logs"`
}

// LogEntry 表示 UI 日志面板中的一条运行事件。
type LogEntry struct {
	Time     time.Time `json:"time"`
	Level    string    `json:"level"`
	Source   string    `json:"source"`
	Message  string    `json:"message"`
	Category string    `json:"category"`
}

// SyncResult 返回 Cloudflare 同步结果和同步过程中的提示。
type SyncResult struct {
	Config   AppConfig `json:"config"`
	Messages []string  `json:"messages"`
}

// TunnelRouteOverviewResult 返回所有 Tunnel 的域名映射总览。
type TunnelRouteOverviewResult struct {
	Routes   []TunnelRouteOverview `json:"routes"`
	Messages []string              `json:"messages"`
}

// TunnelRouteOverview 表示一条带 Tunnel 归属信息的域名映射。
type TunnelRouteOverview struct {
	TunnelID        string `json:"tunnelId"`
	TunnelName      string `json:"tunnelName"`
	TunnelStatus    string `json:"tunnelStatus"`
	Hostname        string `json:"hostname"`
	ServiceProtocol string `json:"serviceProtocol"`
	ServiceHost     string `json:"serviceHost"`
	ServicePort     int    `json:"servicePort"`
	Enabled         bool   `json:"enabled"`
	Source          string `json:"source"`
}

// CloudflareTunnel 映射 Cloudflare Tunnel API 返回的关键字段。
type CloudflareTunnel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Token         string `json:"token"`
	ConnectionsAt string `json:"conns_active_at"`
}

// DNSRecord 映射 Cloudflare DNS 记录 API 返回的关键字段。
type DNSRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
}

// CloudflareAccount 映射 Cloudflare API 返回的账号信息。
type CloudflareAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CloudflareZone 映射 Cloudflare Zone API 返回的关键字段。
type CloudflareZone struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Status  string            `json:"status"`
	Account CloudflareAccount `json:"account"`
}

// CloudflareDiscoveryResult 返回凭据自动发现出的账号、域名和 Tunnel 候选项。
type CloudflareDiscoveryResult struct {
	Config   AppConfig           `json:"config"`
	Accounts []CloudflareAccount `json:"accounts"`
	Zones    []CloudflareZone    `json:"zones"`
	Tunnels  []CloudflareTunnel  `json:"tunnels"`
	Messages []string            `json:"messages"`
}

// NewDefaultConfig 创建首版应用的默认配置。
func NewDefaultConfig() AppConfig {
	return AppConfig{
		AuthType:    defaultAuthType,
		Protocol:    defaultProtocol,
		AutoRestart: true,
		Routes:      []Route{},
	}
}
