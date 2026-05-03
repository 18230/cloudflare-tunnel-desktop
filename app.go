package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App 是 Wails 绑定到前端的应用入口。
type App struct {
	ctx                context.Context
	store              *ConfigStore
	manager            *TunnelManager
	mu                 sync.Mutex
	config             AppConfig
	authType           string
	authEmail          string
	apiToken           string
	tunnelToken        string
	tunnelStatus       string
	installStatus      CloudflaredInstallStatus
	stopHealthMonitor  chan struct{}
	shutdownOnce       sync.Once
	networkFingerprint string
}

// NewApp 创建应用实例并加载本地配置。
func NewApp() *App {
	store, err := NewConfigStore()
	if err != nil {
		store = NewConfigStoreAt("config.json")
	}
	config, err := store.Load()
	if err != nil {
		config = NewDefaultConfig()
	}
	app := &App{
		store:             store,
		config:            config,
		authType:          NormalizeAuthType(config.AuthType),
		authEmail:         strings.TrimSpace(config.AuthEmail),
		apiToken:          strings.TrimSpace(config.APIToken),
		tunnelToken:       strings.TrimSpace(config.TunnelToken),
		stopHealthMonitor: make(chan struct{}),
	}
	app.manager = NewTunnelManager(app.emitLog)
	app.networkFingerprint = currentNetworkFingerprint()
	app.installStatus = app.currentCloudflaredInstallStatus()
	return app
}

// startup 在 Wails 应用启动后保存上下文并启动健康监控。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.manager.addLog("info", "app", "应用已启动，本地配置已加载", "app")
	setupMenuBarTray(a)
	go func() {
		// macOS 菜单栏状态项依赖主 runloop，延迟补偿一次能避免启动早期偶发不显示。
		time.Sleep(2 * time.Second)
		setupMenuBarTray(a)
	}()
	go a.runHealthMonitor()
}

// shutdown 在应用退出时停止后台监控、托盘入口和 cloudflared 进程。
func (a *App) shutdown(ctx context.Context) {
	a.shutdownOnce.Do(func() {
		close(a.stopHealthMonitor)
	})
	teardownMenuBarTray(a)
	_ = a.manager.Stop()
}

// ShowMainWindow 从菜单栏重新显示主窗口，适用于窗口被关闭隐藏后的唤起。
func (a *App) ShowMainWindow() {
	if a.ctx == nil {
		return
	}
	wailsruntime.Show(a.ctx)
	wailsruntime.WindowShow(a.ctx)
}

// HideMainWindow 从菜单栏隐藏主窗口，但保留后台进程和自动恢复能力。
func (a *App) HideMainWindow() {
	if a.ctx == nil {
		return
	}
	wailsruntime.Hide(a.ctx)
}

// QuitApplication 从菜单栏执行完整退出，会触发 shutdown 停止后台进程。
func (a *App) QuitApplication() {
	if a.ctx == nil {
		return
	}
	wailsruntime.Quit(a.ctx)
}

// LoadConfig 返回当前本地配置。
func (a *App) LoadConfig() (AppConfig, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.config, nil
}

// SaveSettings 保存 Cloudflare 基础设置，并保留已有凭据。
func (a *App) SaveSettings(input SettingsInput) (AppConfig, error) {
	normalized, err := NormalizeSettings(input)
	if err != nil {
		return AppConfig{}, err
	}
	a.mu.Lock()
	config := a.config
	config.AccountID = normalized.AccountID
	config.ZoneID = normalized.ZoneID
	config.RootDomain = normalized.RootDomain
	config.TunnelID = normalized.TunnelID
	config.TunnelName = normalized.TunnelName
	config.Protocol = normalized.Protocol
	config.AutoRestart = normalized.AutoRestart
	config.AuthType = a.authType
	config.AuthEmail = a.authEmail
	config.APIToken = a.apiToken
	config.TunnelToken = a.tunnelToken
	a.config = config
	a.mu.Unlock()

	if err := a.store.Save(config); err != nil {
		return AppConfig{}, err
	}
	a.manager.SetAutoRestart(config.AutoRestart)
	a.manager.addLog("info", "config", "基础配置已保存", "config")
	return config, nil
}

// SetCredentials 将 API Token 和可选 Tunnel Token 明文保存到本地配置文件。
func (a *App) SetCredentials(input CredentialsInput) (RuntimeStatus, error) {
	a.mu.Lock()
	a.authType = NormalizeAuthType(input.AuthType)
	a.authEmail = strings.TrimSpace(input.AuthEmail)
	a.apiToken = strings.TrimSpace(input.APIToken)
	a.tunnelToken = strings.TrimSpace(input.TunnelToken)
	a.config.AuthType = a.authType
	a.config.AuthEmail = a.authEmail
	a.config.APIToken = a.apiToken
	a.config.TunnelToken = a.tunnelToken
	config := a.config
	a.mu.Unlock()
	if err := a.store.Save(config); err != nil {
		return RuntimeStatus{}, err
	}
	a.manager.addLog("info", "auth", "凭据已保存到本地配置文件", "auth")
	return a.GetStatus(), nil
}

// CreateTunnel 通过 Cloudflare API 创建远程管理 Tunnel 并保存 Tunnel ID。
func (a *App) CreateTunnel(name string) (AppConfig, error) {
	a.mu.Lock()
	config := a.config
	auth := a.currentCloudflareAuthLocked()
	a.mu.Unlock()
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSpace(config.TunnelName)
	}
	if name == "" {
		name = "desktop-tunnel"
	}
	if config.AccountID == "" || !cloudflareIDPattern.MatchString(config.AccountID) {
		return AppConfig{}, fmt.Errorf("请先保存有效的 Account ID")
	}
	client := NewCloudflareClientWithAuth(auth)
	tunnel, err := client.CreateTunnel(context.Background(), config.AccountID, name)
	if err != nil {
		return AppConfig{}, err
	}
	if tunnel.ID == "" {
		return AppConfig{}, fmt.Errorf("Cloudflare 创建 Tunnel 成功但未返回 Tunnel ID")
	}

	a.mu.Lock()
	a.config.TunnelID = tunnel.ID
	a.config.TunnelName = name
	if tunnel.Token != "" {
		a.tunnelToken = tunnel.Token
		a.config.TunnelToken = tunnel.Token
	}
	a.config.AuthType = a.authType
	a.config.AuthEmail = a.authEmail
	a.config.APIToken = a.apiToken
	config = a.config
	a.mu.Unlock()
	if err := a.store.Save(config); err != nil {
		return AppConfig{}, err
	}
	a.manager.addLog("info", "cloudflare", "Tunnel 已创建: "+tunnel.ID, "api")
	return config, nil
}

// ListTunnels 获取当前账号下已有的 Cloudflare Tunnel。
func (a *App) ListTunnels() ([]CloudflareTunnel, error) {
	a.mu.Lock()
	config := a.config
	auth := a.currentCloudflareAuthLocked()
	a.mu.Unlock()
	if config.AccountID == "" || !cloudflareIDPattern.MatchString(config.AccountID) {
		return nil, fmt.Errorf("请先保存有效的 Account ID")
	}
	client := NewCloudflareClientWithAuth(auth)
	tunnels, err := client.ListTunnels(context.Background(), config.AccountID)
	if err != nil {
		return nil, err
	}
	a.manager.addLog("info", "cloudflare", fmt.Sprintf("已获取 %d 个 Tunnel", len(tunnels)), "api")
	return tunnels, nil
}

// BindTunnel 将已有 Tunnel 设为当前应用管理的 Tunnel。
func (a *App) BindTunnel(tunnel CloudflareTunnel) (AppConfig, error) {
	tunnel.ID = strings.TrimSpace(tunnel.ID)
	tunnel.Name = strings.TrimSpace(tunnel.Name)
	if !uuidPattern.MatchString(tunnel.ID) {
		return AppConfig{}, fmt.Errorf("Tunnel ID 必须是 UUID 格式")
	}
	a.mu.Lock()
	a.config.TunnelID = tunnel.ID
	a.config.TunnelName = tunnel.Name
	if tunnel.Token != "" {
		a.tunnelToken = tunnel.Token
		a.config.TunnelToken = tunnel.Token
	}
	config := a.config
	a.mu.Unlock()
	if err := a.store.Save(config); err != nil {
		return AppConfig{}, err
	}
	a.manager.addLog("info", "cloudflare", "已设为当前 Tunnel "+tunnel.ID, "api")
	return config, nil
}

// DeleteTunnel 删除 Cloudflare 上的指定 Tunnel，并可选清理指向它的 DNS 记录。
func (a *App) DeleteTunnel(tunnelID string, deleteDNS bool) (AppConfig, error) {
	tunnelID = strings.TrimSpace(tunnelID)
	if !uuidPattern.MatchString(tunnelID) {
		return AppConfig{}, fmt.Errorf("Tunnel ID 必须是 UUID 格式")
	}
	a.mu.Lock()
	config := a.config
	auth := a.currentCloudflareAuthLocked()
	a.mu.Unlock()
	if config.AccountID == "" || !cloudflareIDPattern.MatchString(config.AccountID) {
		return AppConfig{}, fmt.Errorf("请先保存有效的 Account ID")
	}
	if deleteDNS && !cloudflareIDPattern.MatchString(config.ZoneID) {
		return AppConfig{}, fmt.Errorf("请先保存有效的 Zone ID，才能同时删除 DNS 记录")
	}
	client := NewCloudflareClientWithAuth(auth)
	deletedDNSCount := 0
	if deleteDNS {
		count, err := client.DeleteTunnelDNSRecords(context.Background(), config.ZoneID, tunnelID)
		if err != nil {
			return AppConfig{}, err
		}
		deletedDNSCount = count
	}
	if err := client.DeleteTunnel(context.Background(), config.AccountID, tunnelID); err != nil {
		return AppConfig{}, err
	}
	a.mu.Lock()
	if a.config.TunnelID == tunnelID {
		a.config.TunnelID = ""
		a.config.TunnelName = ""
		a.config.TunnelToken = ""
		a.tunnelToken = ""
	}
	config = a.config
	a.mu.Unlock()
	if err := a.store.Save(config); err != nil {
		return AppConfig{}, err
	}
	a.manager.addLog("warn", "cloudflare", "已删除 Tunnel "+tunnelID, "api")
	if deleteDNS {
		a.manager.addLog("warn", "cloudflare", fmt.Sprintf("已删除 %d 条指向该 Tunnel 的 DNS 记录", deletedDNSCount), "dns")
	}
	return config, nil
}

// FetchZones 根据当前 API Token、Zone ID 或 Account ID 获取可选根域名。
func (a *App) FetchZones() ([]CloudflareZone, error) {
	a.mu.Lock()
	config := a.config
	auth := a.currentCloudflareAuthLocked()
	a.mu.Unlock()
	if auth.Type == authTypeGlobalKey && (auth.Email == "" || auth.Key == "") {
		return nil, fmt.Errorf("请先在本地凭据中输入 Cloudflare 邮箱和 Global API Key")
	}
	if auth.Type == authTypeAPIToken && auth.Key == "" {
		return nil, fmt.Errorf("请先在本地凭据中输入 Cloudflare API Token")
	}
	client := NewCloudflareClientWithAuth(auth)
	if strings.TrimSpace(config.ZoneID) != "" {
		if !cloudflareIDPattern.MatchString(config.ZoneID) {
			return nil, fmt.Errorf("Zone ID 必须是 32 位十六进制字符串")
		}
		zone, err := client.GetZone(context.Background(), config.ZoneID)
		if err != nil {
			return nil, err
		}
		a.manager.addLog("info", "cloudflare", "已通过 Zone ID 获取根域名 "+zone.Name, "api")
		return []CloudflareZone{zone}, nil
	}
	if strings.TrimSpace(config.AccountID) != "" && !cloudflareIDPattern.MatchString(config.AccountID) {
		return nil, fmt.Errorf("Account ID 必须是 32 位十六进制字符串")
	}
	zones, err := client.ListZones(context.Background(), config.AccountID)
	if err != nil {
		return nil, err
	}
	if len(zones) == 0 {
		return nil, fmt.Errorf("没有获取到可用根域名，请确认 API Token 具备 Zone Read 权限")
	}
	a.manager.addLog("info", "cloudflare", fmt.Sprintf("已获取 %d 个根域名", len(zones)), "api")
	return zones, nil
}

// AddRoute 新增一条公开域名到本地服务的映射。
func (a *App) AddRoute(input RouteInput) (AppConfig, error) {
	a.mu.Lock()
	config := a.config
	a.mu.Unlock()
	route, err := NormalizeRoute(input, config.RootDomain)
	if err != nil {
		return AppConfig{}, err
	}
	route.ID = newRouteID()
	if route.ServiceHost == "" {
		route.ServiceHost = defaultServiceHost
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	for _, existing := range a.config.Routes {
		if existing.Hostname == route.Hostname {
			return AppConfig{}, fmt.Errorf("公开域名已存在")
		}
	}
	a.config.Routes = append(a.config.Routes, route)
	if err := a.store.Save(a.config); err != nil {
		return AppConfig{}, err
	}
	a.manager.addLog("info", "route", "已添加映射 "+route.Hostname+" -> "+BuildServiceURL(route), "config")
	return a.config, nil
}

// UpdateRoute 更新一条已有映射。
func (a *App) UpdateRoute(input RouteInput) (AppConfig, error) {
	input.ID = strings.TrimSpace(input.ID)
	if input.ID == "" {
		return AppConfig{}, fmt.Errorf("缺少路由 ID")
	}
	a.mu.Lock()
	config := a.config
	a.mu.Unlock()
	route, err := NormalizeRoute(input, config.RootDomain)
	if err != nil {
		return AppConfig{}, err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	index := -1
	for i, existing := range a.config.Routes {
		if existing.ID == route.ID {
			index = i
			continue
		}
		if existing.Hostname == route.Hostname {
			return AppConfig{}, fmt.Errorf("公开域名已存在")
		}
	}
	if index < 0 {
		return AppConfig{}, fmt.Errorf("未找到要更新的路由")
	}
	a.config.Routes[index] = route
	if err := a.store.Save(a.config); err != nil {
		return AppConfig{}, err
	}
	a.manager.addLog("info", "route", "已更新映射 "+route.Hostname, "config")
	return a.config, nil
}

// RemoveRoute 删除一条映射，并可选删除对应 DNS 记录。
func (a *App) RemoveRoute(routeID string, deleteDNS bool) (AppConfig, error) {
	routeID = strings.TrimSpace(routeID)
	if routeID == "" {
		return AppConfig{}, fmt.Errorf("缺少路由 ID")
	}

	a.mu.Lock()
	config := a.config
	auth := a.currentCloudflareAuthLocked()
	var removed *Route
	for _, route := range config.Routes {
		if route.ID == routeID {
			copyRoute := route
			removed = &copyRoute
			break
		}
	}
	a.mu.Unlock()
	if removed == nil {
		return AppConfig{}, fmt.Errorf("未找到要删除的路由")
	}

	if deleteDNS && auth.Key != "" && config.ZoneID != "" && config.TunnelID != "" {
		client := NewCloudflareClientWithAuth(auth)
		deleted, err := client.DeleteTunnelDNSRecord(context.Background(), config.ZoneID, config.TunnelID, removed.Hostname)
		if err != nil {
			return AppConfig{}, err
		}
		if deleted {
			a.manager.addLog("info", "cloudflare", "已删除 DNS 记录 "+removed.Hostname, "dns")
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	routes := make([]Route, 0, len(a.config.Routes))
	for _, route := range a.config.Routes {
		if route.ID != routeID {
			routes = append(routes, route)
		}
	}
	a.config.Routes = routes
	if err := a.store.Save(a.config); err != nil {
		return AppConfig{}, err
	}
	a.manager.addLog("info", "route", "已删除映射 "+removed.Hostname, "config")
	return a.config, nil
}

// SyncRoutes 将当前映射同步到 Cloudflare Tunnel 配置和 DNS。
func (a *App) SyncRoutes() (SyncResult, error) {
	a.mu.Lock()
	config := a.config
	auth := a.currentCloudflareAuthLocked()
	a.mu.Unlock()
	if err := ValidateConfigForSync(config); err != nil {
		return SyncResult{}, err
	}
	client := NewCloudflareClientWithAuth(auth)
	if err := client.PutTunnelConfiguration(context.Background(), config.AccountID, config.TunnelID, config.Routes); err != nil {
		return SyncResult{}, err
	}
	messages := []string{"Tunnel ingress 配置已同步"}
	for _, route := range config.Routes {
		if !route.Enabled {
			continue
		}
		if _, err := client.EnsureTunnelDNSRecord(context.Background(), config.ZoneID, config.TunnelID, route.Hostname); err != nil {
			return SyncResult{}, err
		}
		messages = append(messages, "DNS 已同步: "+route.Hostname)
	}
	if err := a.store.Save(config); err != nil {
		return SyncResult{}, err
	}
	a.manager.addLog("info", "cloudflare", "Cloudflare 配置同步完成", "api")
	return SyncResult{Config: config, Messages: messages}, nil
}

// PullRoutesFromCloudflare 从远端 Tunnel ingress 和 DNS 记录读取已有映射并保存到本地。
func (a *App) PullRoutesFromCloudflare() (SyncResult, error) {
	a.mu.Lock()
	config := a.config
	auth := a.currentCloudflareAuthLocked()
	a.mu.Unlock()
	if !cloudflareIDPattern.MatchString(config.AccountID) {
		return SyncResult{}, fmt.Errorf("请先配置有效的 Account ID")
	}
	if !uuidPattern.MatchString(config.TunnelID) {
		return SyncResult{}, fmt.Errorf("请先绑定或填写有效的 Tunnel ID")
	}
	if config.RootDomain != "" && !isValidHostname(config.RootDomain) {
		return SyncResult{}, fmt.Errorf("根域名格式不正确")
	}

	client := NewCloudflareClientWithAuth(auth)
	remoteConfig, err := client.GetTunnelConfiguration(context.Background(), config.AccountID, config.TunnelID)
	if err != nil {
		return SyncResult{}, err
	}
	routes, skipped := RoutesFromTunnelConfiguration(remoteConfig, config.RootDomain)

	dnsOnlyCount := 0
	var dnsReadErr error
	if cloudflareIDPattern.MatchString(config.ZoneID) {
		dnsRecords, err := client.ListTunnelDNSRecords(context.Background(), config.ZoneID, config.TunnelID)
		if err != nil {
			dnsReadErr = err
		} else {
			knownHostnames := make(map[string]struct{}, len(routes))
			for _, route := range routes {
				knownHostnames[route.Hostname] = struct{}{}
			}
			for _, record := range dnsRecords {
				hostname := normalizeHostname(record.Name)
				if _, exists := knownHostnames[hostname]; exists {
					continue
				}
				route, ok := RouteFromTunnelDNSRecord(record, config.RootDomain)
				if !ok {
					skipped++
					continue
				}
				knownHostnames[route.Hostname] = struct{}{}
				routes = append(routes, route)
				dnsOnlyCount++
			}
		}
	}
	if len(routes) == 0 && skipped > 0 && dnsReadErr == nil {
		message := fmt.Sprintf("Cloudflare 上没有本应用可导入的 HTTP/HTTPS 映射，已跳过 %d 条暂不支持的规则", skipped)
		a.manager.addLog("warn", "cloudflare", message, "api")
		return SyncResult{Config: config, Messages: []string{message}}, nil
	}
	if len(routes) == 0 && dnsReadErr != nil {
		message := "Tunnel ingress 没有可显示映射；DNS 记录读取失败，已保留本地列表: " + dnsReadErr.Error()
		a.manager.addLog("warn", "cloudflare", message, "api")
		return SyncResult{Config: config, Messages: []string{message}}, nil
	}

	existingIDs := make(map[string]string, len(config.Routes))
	for _, route := range config.Routes {
		existingIDs[route.Hostname] = route.ID
	}
	for i := range routes {
		if id := existingIDs[routes[i].Hostname]; id != "" {
			routes[i].ID = id
			continue
		}
		routes[i].ID = newRouteID()
	}

	a.mu.Lock()
	a.config.Routes = routes
	config = a.config
	a.mu.Unlock()
	if err := a.store.Save(config); err != nil {
		return SyncResult{}, err
	}

	messages := []string{fmt.Sprintf("已从 Cloudflare 读取 %d 条映射", len(routes))}
	if len(routes) == 0 {
		messages[0] = "Cloudflare 当前没有可显示映射，已清空本地列表"
	}
	if dnsOnlyCount > 0 {
		messages = append(messages, fmt.Sprintf("其中 %d 条只在 DNS 中发现，已按域名前缀推断端口并设为停用，请确认后启用", dnsOnlyCount))
	}
	if skipped > 0 {
		messages = append(messages, fmt.Sprintf("已跳过 %d 条暂不支持的 SSH/TCP/path 或重复规则", skipped))
	}
	if dnsReadErr != nil {
		messages = append(messages, "DNS 记录读取失败: "+dnsReadErr.Error())
	}
	a.manager.addLog("info", "cloudflare", messages[0], "api")
	return SyncResult{Config: config, Messages: messages}, nil
}

// ListAllTunnelRoutes 读取当前账号下所有 Tunnel 的域名映射并附带归属信息。
func (a *App) ListAllTunnelRoutes() (TunnelRouteOverviewResult, error) {
	a.mu.Lock()
	config := a.config
	auth := a.currentCloudflareAuthLocked()
	a.mu.Unlock()
	if !cloudflareIDPattern.MatchString(config.AccountID) {
		return TunnelRouteOverviewResult{}, fmt.Errorf("请先配置有效的 Account ID")
	}
	client := NewCloudflareClientWithAuth(auth)
	tunnels, err := client.ListTunnels(context.Background(), config.AccountID)
	if err != nil {
		return TunnelRouteOverviewResult{}, err
	}

	cnameRecords := []DNSRecord{}
	messages := []string{}
	if cloudflareIDPattern.MatchString(config.ZoneID) {
		records, err := client.ListZoneCNAMERecords(context.Background(), config.ZoneID)
		if err != nil {
			messages = append(messages, "DNS 记录读取失败: "+err.Error())
		} else {
			cnameRecords = records
		}
	} else {
		messages = append(messages, "未配置有效 Zone ID，已跳过 DNS-only 映射读取")
	}

	overview := []TunnelRouteOverview{}
	for _, tunnel := range tunnels {
		items, skipped, err := a.readTunnelRouteOverview(client, config, tunnel, cnameRecords)
		if err != nil {
			messages = append(messages, fmt.Sprintf("%s 读取失败: %s", tunnelDisplayName(tunnel), err.Error()))
			continue
		}
		if skipped > 0 {
			messages = append(messages, fmt.Sprintf("%s 已跳过 %d 条暂不支持规则", tunnelDisplayName(tunnel), skipped))
		}
		overview = append(overview, items...)
	}
	sort.SliceStable(overview, func(i, j int) bool {
		if overview[i].TunnelName != overview[j].TunnelName {
			return overview[i].TunnelName < overview[j].TunnelName
		}
		return overview[i].Hostname < overview[j].Hostname
	})
	a.manager.addLog("info", "cloudflare", fmt.Sprintf("已读取所有 Tunnel 的 %d 条域名映射", len(overview)), "api")
	return TunnelRouteOverviewResult{Routes: overview, Messages: messages}, nil
}

// readTunnelRouteOverview 读取单个 Tunnel 的 ingress 与 DNS-only 映射。
func (a *App) readTunnelRouteOverview(client *CloudflareClient, config AppConfig, tunnel CloudflareTunnel, cnameRecords []DNSRecord) ([]TunnelRouteOverview, int, error) {
	remoteConfig, err := client.GetTunnelConfiguration(context.Background(), config.AccountID, tunnel.ID)
	if err != nil {
		return nil, 0, err
	}
	routes, skipped := RoutesFromTunnelConfiguration(remoteConfig, config.RootDomain)
	seenHostnames := make(map[string]struct{}, len(routes))
	items := make([]TunnelRouteOverview, 0, len(routes))
	for _, route := range routes {
		seenHostnames[route.Hostname] = struct{}{}
		items = append(items, routeOverviewFromRoute(tunnel, route, "ingress"))
	}

	target := tunnelDNSContent(tunnel.ID)
	for _, record := range cnameRecords {
		if !strings.EqualFold(record.Content, target) {
			continue
		}
		hostname := normalizeHostname(record.Name)
		if _, exists := seenHostnames[hostname]; exists {
			continue
		}
		route, ok := RouteFromTunnelDNSRecord(record, config.RootDomain)
		if !ok {
			skipped++
			continue
		}
		seenHostnames[route.Hostname] = struct{}{}
		items = append(items, routeOverviewFromRoute(tunnel, route, "dns"))
	}
	return items, skipped, nil
}

// routeOverviewFromRoute 给普通映射补充 Tunnel 归属信息。
func routeOverviewFromRoute(tunnel CloudflareTunnel, route Route, source string) TunnelRouteOverview {
	return TunnelRouteOverview{
		TunnelID:        tunnel.ID,
		TunnelName:      tunnel.Name,
		TunnelStatus:    tunnel.Status,
		Hostname:        route.Hostname,
		ServiceProtocol: route.ServiceProtocol,
		ServiceHost:     route.ServiceHost,
		ServicePort:     route.ServicePort,
		Enabled:         route.Enabled,
		Source:          source,
	}
}

// tunnelDisplayName 返回适合 UI 提示的 Tunnel 名称。
func tunnelDisplayName(tunnel CloudflareTunnel) string {
	if strings.TrimSpace(tunnel.Name) != "" {
		return strings.TrimSpace(tunnel.Name)
	}
	return tunnel.ID
}

// findExecutableWithFallback 查找命令，兼容桌面进程 PATH 不完整的情况。
func findExecutableWithFallback(name string, fallbackPaths []string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	for _, path := range fallbackPaths {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("未找到 %s", name)
}

// StartTunnel 启动本地 cloudflared 连接。
func (a *App) StartTunnel() (RuntimeStatus, error) {
	a.mu.Lock()
	config := a.config
	a.mu.Unlock()
	if err := ValidateConfigForRun(config); err != nil {
		return RuntimeStatus{}, err
	}
	token, err := a.resolveTunnelToken(config)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if err := a.manager.Start(token, config.TunnelID, config.Protocol, config.AutoRestart); err != nil {
		return RuntimeStatus{}, err
	}
	return a.GetStatus(), nil
}

// StopTunnel 停止本地 cloudflared 连接。
func (a *App) StopTunnel() (RuntimeStatus, error) {
	if err := a.manager.Stop(); err != nil {
		return RuntimeStatus{}, err
	}
	return a.GetStatus(), nil
}

// RestartTunnel 使用当前配置重启本地 cloudflared。
func (a *App) RestartTunnel() (RuntimeStatus, error) {
	a.mu.Lock()
	config := a.config
	a.mu.Unlock()
	if err := ValidateConfigForRun(config); err != nil {
		return RuntimeStatus{}, err
	}
	token, err := a.resolveTunnelToken(config)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if err := a.manager.Restart(token, config.TunnelID, config.Protocol, config.AutoRestart); err != nil {
		return RuntimeStatus{}, err
	}
	return a.GetStatus(), nil
}

// RefreshStatus 主动刷新 Cloudflare Tunnel 状态。
func (a *App) RefreshStatus() (RuntimeStatus, error) {
	a.mu.Lock()
	config := a.config
	auth := a.currentCloudflareAuthLocked()
	a.mu.Unlock()
	if auth.Key != "" && cloudflareIDPattern.MatchString(config.AccountID) && uuidPattern.MatchString(config.TunnelID) {
		client := NewCloudflareClientWithAuth(auth)
		tunnel, err := client.GetTunnel(context.Background(), config.AccountID, config.TunnelID)
		if err != nil {
			a.manager.addLog("warn", "cloudflare", "刷新 Tunnel 状态失败: "+err.Error(), "api")
		} else {
			a.mu.Lock()
			a.tunnelStatus = tunnel.Status
			a.mu.Unlock()
			if (tunnel.Status == "down" || tunnel.Status == "degraded") && config.AutoRestart {
				a.manager.TriggerRecovery("Tunnel 状态为 " + tunnel.Status)
			}
		}
	}
	return a.GetStatus(), nil
}

// GetStatus 返回前端 Dashboard 使用的聚合状态。
func (a *App) GetStatus() RuntimeStatus {
	managerStatus := a.manager.Status()
	a.mu.Lock()
	defer a.mu.Unlock()
	managerStatus.Configured = a.config.AccountID != "" && a.config.ZoneID != "" && a.config.RootDomain != ""
	managerStatus.AuthType = NormalizeAuthType(a.authType)
	managerStatus.APITokenSet = strings.TrimSpace(a.apiToken) != ""
	managerStatus.TunnelTokenSet = strings.TrimSpace(a.tunnelToken) != ""
	managerStatus.Protocol = a.config.Protocol
	managerStatus.TunnelStatus = a.tunnelStatus
	managerStatus.AutoRestart = a.config.AutoRestart
	managerStatus.RouteCount = len(a.config.Routes)
	managerStatus.HasTunnelID = a.config.TunnelID != ""
	return managerStatus
}

// GetCloudflaredInstallStatus 返回 cloudflared 安装检测和安装进度。
func (a *App) GetCloudflaredInstallStatus() CloudflaredInstallStatus {
	a.mu.Lock()
	installing := a.installStatus.Installing
	status := a.installStatus
	a.mu.Unlock()
	if status.Logs == nil {
		status.Logs = []string{}
	}
	if installing {
		return status
	}
	return a.currentCloudflaredInstallStatus()
}

// InstallCloudflared 根据当前系统异步安装 cloudflared。
func (a *App) InstallCloudflared() (CloudflaredInstallStatus, error) {
	current := a.currentCloudflaredInstallStatus()
	if current.Installed {
		return current, nil
	}
	installer, err := cloudflaredInstallCommand()
	if err != nil {
		return CloudflaredInstallStatus{}, err
	}

	a.mu.Lock()
	if a.installStatus.Installing {
		status := a.installStatus
		a.mu.Unlock()
		return status, nil
	}
	a.installStatus = CloudflaredInstallStatus{
		Installing: true,
		Status:     "准备安装 cloudflared",
		Logs:       append([]string{}, installer.Logs...),
	}
	status := a.installStatus
	a.mu.Unlock()

	go a.runCloudflaredInstall(installer)
	return status, nil
}

// currentCloudflaredInstallStatus 检测本机 cloudflared 是否已经可用。
func (a *App) currentCloudflaredInstallStatus() CloudflaredInstallStatus {
	a.manager.refreshCloudflaredInfo()
	status := a.manager.Status()
	result := CloudflaredInstallStatus{
		Installed: status.CloudflaredPath != "",
		Path:      status.CloudflaredPath,
		Version:   status.CloudflaredVersion,
		Logs:      []string{},
	}
	if result.Installed {
		result.Status = "cloudflared 已安装"
	} else {
		result.Status = "cloudflared 未安装"
	}
	a.mu.Lock()
	if !a.installStatus.Installing {
		a.installStatus = result
	}
	a.mu.Unlock()
	return result
}

// cloudflaredInstaller 描述当前系统可用的 cloudflared 安装命令。
type cloudflaredInstaller struct {
	Name       string
	Executable string
	Args       []string
	Logs       []string
}

// cloudflaredInstallCommand 根据当前系统选择自动安装 cloudflared 的命令。
func cloudflaredInstallCommand() (cloudflaredInstaller, error) {
	return cloudflaredInstallCommandForOS(runtime.GOOS)
}

// cloudflaredInstallCommandForOS 为指定系统选择安装命令，便于单元测试覆盖跨平台分支。
func cloudflaredInstallCommandForOS(goos string) (cloudflaredInstaller, error) {
	switch goos {
	case "darwin":
		brewPath, err := findExecutableWithFallback("brew", []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"})
		if err != nil {
			return cloudflaredInstaller{}, fmt.Errorf("未找到 Homebrew，请先安装 Homebrew 后再自动安装 cloudflared")
		}
		return cloudflaredInstaller{
			Name:       "Homebrew",
			Executable: brewPath,
			Args:       []string{"install", "cloudflared"},
			Logs:       []string{"使用 Homebrew 安装 cloudflared"},
		}, nil
	case "windows":
		return windowsCloudflaredInstallCommand()
	default:
		return cloudflaredInstaller{}, fmt.Errorf("当前自动安装支持 macOS 和 Windows，请手动安装 cloudflared 后再重试")
	}
}

// windowsCloudflaredInstallCommand 按 Windows 常见包管理器优先级选择安装命令。
func windowsCloudflaredInstallCommand() (cloudflaredInstaller, error) {
	if wingetPath, err := exec.LookPath("winget"); err == nil {
		return cloudflaredInstaller{
			Name:       "winget",
			Executable: wingetPath,
			Args: []string{
				"install",
				"--id", "Cloudflare.cloudflared",
				"--exact",
				"--silent",
				"--accept-package-agreements",
				"--accept-source-agreements",
			},
			Logs: []string{"使用 winget 安装 cloudflared"},
		}, nil
	}
	if scoopPath, err := exec.LookPath("scoop"); err == nil {
		return cloudflaredInstaller{
			Name:       "scoop",
			Executable: scoopPath,
			Args:       []string{"install", "cloudflared"},
			Logs:       []string{"未找到 winget，改用 scoop 安装 cloudflared"},
		}, nil
	}
	if chocoPath, err := exec.LookPath("choco"); err == nil {
		return cloudflaredInstaller{
			Name:       "Chocolatey",
			Executable: chocoPath,
			Args:       []string{"install", "cloudflared", "-y"},
			Logs:       []string{"未找到 winget 或 scoop，改用 Chocolatey 安装 cloudflared"},
		}, nil
	}
	return cloudflaredInstaller{}, fmt.Errorf("未找到 winget、scoop 或 choco，请先安装任一 Windows 包管理器，或手动安装 cloudflared.exe 后再重试")
}

// runCloudflaredInstall 执行系统安装命令并持续记录输出。
func (a *App) runCloudflaredInstall(installer cloudflaredInstaller) {
	cmd := exec.Command(installer.Executable, installer.Args...)
	configureBackgroundCommand(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.finishCloudflaredInstall("连接安装输出失败: "+err.Error(), false)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		a.finishCloudflaredInstall("连接安装错误输出失败: "+err.Error(), false)
		return
	}
	if err := cmd.Start(); err != nil {
		a.finishCloudflaredInstall("启动安装失败: "+err.Error(), false)
		return
	}
	a.appendCloudflaredInstallLog(installer.Name + " 安装进程已启动")
	done := make(chan struct{}, 2)
	go a.scanInstallPipe(stdout, done)
	go a.scanInstallPipe(stderr, done)
	<-done
	<-done
	if err := cmd.Wait(); err != nil {
		a.finishCloudflaredInstall("安装失败: "+err.Error(), false)
		return
	}
	a.manager.refreshCloudflaredInfo()
	status := a.manager.Status()
	if status.CloudflaredPath == "" {
		a.finishCloudflaredInstall("安装完成但仍未找到 cloudflared", false)
		return
	}
	a.mu.Lock()
	a.installStatus = CloudflaredInstallStatus{
		Installed: true,
		Path:      status.CloudflaredPath,
		Version:   status.CloudflaredVersion,
		Status:    "cloudflared 安装完成",
		Logs:      append(a.installStatus.Logs, "cloudflared 安装完成"),
	}
	a.mu.Unlock()
	a.manager.addLog("info", "cloudflared", "cloudflared 安装完成", "install")
}

// scanInstallPipe 读取安装命令输出并记录为安装进度。
func (a *App) scanInstallPipe(pipe any, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()
	reader, ok := pipe.(interface {
		Read([]byte) (int, error)
	})
	if !ok {
		return
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		a.appendCloudflaredInstallLog(scanner.Text())
	}
}

// appendCloudflaredInstallLog 追加安装日志，限制数量避免 UI 过长。
func (a *App) appendCloudflaredInstallLog(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	a.mu.Lock()
	a.installStatus.Status = line
	a.installStatus.Logs = append(a.installStatus.Logs, line)
	if len(a.installStatus.Logs) > 80 {
		a.installStatus.Logs = a.installStatus.Logs[len(a.installStatus.Logs)-80:]
	}
	a.mu.Unlock()
}

// finishCloudflaredInstall 结束安装流程并保存错误或最终状态。
func (a *App) finishCloudflaredInstall(message string, installed bool) {
	a.mu.Lock()
	a.installStatus.Installing = false
	a.installStatus.Installed = installed
	a.installStatus.Status = message
	if !installed {
		a.installStatus.Error = message
	}
	a.installStatus.Logs = append(a.installStatus.Logs, message)
	a.mu.Unlock()
	if installed {
		a.manager.addLog("info", "cloudflared", message, "install")
	} else {
		a.manager.addLog("error", "cloudflared", message, "install")
	}
}

// GetLogs 返回最近的应用和 cloudflared 日志。
func (a *App) GetLogs() []LogEntry {
	return a.manager.Logs()
}

// resolveTunnelToken 获取当前启动所需的 Tunnel Token，优先使用本地配置中已加载的 token。
func (a *App) resolveTunnelToken(config AppConfig) (string, error) {
	a.mu.Lock()
	if a.tunnelToken != "" {
		token := a.tunnelToken
		a.mu.Unlock()
		return token, nil
	}
	auth := a.currentCloudflareAuthLocked()
	a.mu.Unlock()
	client := NewCloudflareClientWithAuth(auth)
	token, err := client.GetTunnelToken(context.Background(), config.AccountID, config.TunnelID)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.tunnelToken = token
	a.config.TunnelToken = token
	config = a.config
	a.mu.Unlock()
	if err := a.store.Save(config); err != nil {
		return "", err
	}
	a.manager.addLog("info", "cloudflare", "已通过 API 获取 Tunnel Token，并保存到本地配置", "auth")
	return token, nil
}

// currentCloudflareAuthLocked 基于当前配置生成 Cloudflare API 认证信息；调用方需持有 a.mu。
func (a *App) currentCloudflareAuthLocked() CloudflareAuth {
	return CloudflareAuth{
		Type:  NormalizeAuthType(a.authType),
		Email: strings.TrimSpace(a.authEmail),
		Key:   strings.TrimSpace(a.apiToken),
	}
}

// runHealthMonitor 监控网络指纹变化并定期刷新远程 Tunnel 状态。
func (a *App) runHealthMonitor() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	refreshCount := 0
	for {
		select {
		case <-a.stopHealthMonitor:
			return
		case <-ticker.C:
			current := currentNetworkFingerprint()
			a.mu.Lock()
			previous := a.networkFingerprint
			a.networkFingerprint = current
			a.mu.Unlock()
			if previous != "" && current != previous {
				a.manager.TriggerRecovery("网络变化")
			}
			refreshCount++
			if refreshCount%3 == 0 {
				_, _ = a.RefreshStatus()
			}
		}
	}
}

// emitLog 将后端日志事件推送给前端。
func (a *App) emitLog(entry LogEntry) {
	if a.ctx == nil {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "app:log", entry)
}

// newRouteID 生成短路由 ID，避免把 hostname 当作内部主键。
func newRouteID() string {
	var data [8]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("route-%d", time.Now().UnixNano())
	}
	return "route-" + hex.EncodeToString(data[:])
}

// currentNetworkFingerprint 生成本机非回环网络地址指纹，用于识别切网。
func currentNetworkFingerprint() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "unknown"
	}
	parts := []string{}
	for _, item := range interfaces {
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := item.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			parts = append(parts, item.Name+"="+address.String())
		}
	}
	if len(parts) == 0 {
		return "offline"
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}
