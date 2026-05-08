package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const cloudflareAPIBaseURL = "https://api.cloudflare.com/client/v4"

// CloudflareClient 封装本应用需要的 Cloudflare API 调用。
type CloudflareClient struct {
	baseURL    string
	auth       CloudflareAuth
	httpClient *http.Client
}

// CloudflareAuth 描述 Cloudflare API 的两种认证方式。
type CloudflareAuth struct {
	Type  string
	Email string
	Key   string
}

type cloudflareAPIMessage struct {
	Code             int    `json:"code"`
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
}

type cloudflareAPIEnvelope struct {
	Success    bool                   `json:"success"`
	Errors     []cloudflareAPIMessage `json:"errors"`
	Messages   []cloudflareAPIMessage `json:"messages"`
	Result     json.RawMessage        `json:"result"`
	ResultInfo cloudflareResultInfo   `json:"result_info"`
}

type cloudflareResultInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Count      int `json:"count"`
	TotalCount int `json:"total_count"`
	TotalPages int `json:"total_pages"`
}

type tunnelConfigurationPayload struct {
	Config tunnelConfiguration `json:"config"`
}

type tunnelConfigurationResponse struct {
	Config tunnelConfiguration `json:"config"`
}

type tunnelConfiguration struct {
	Ingress []tunnelIngressRule `json:"ingress"`
}

type tunnelIngressRule struct {
	Hostname      string         `json:"hostname,omitempty"`
	Service       string         `json:"service"`
	Path          string         `json:"path,omitempty"`
	OriginRequest map[string]any `json:"originRequest,omitempty"`
}

// NewCloudflareClient 创建指向 Cloudflare 官方 API 的客户端；email 和 globalKey 来自 Global API Key 页面。
func NewCloudflareClient(email string, globalKey string) *CloudflareClient {
	return NewCloudflareClientWithBaseURL(cloudflareAPIBaseURL, email, globalKey)
}

// NewCloudflareClientWithBaseURL 创建可指定 API 地址的客户端，测试中用于 httptest。
func NewCloudflareClientWithBaseURL(baseURL string, email string, globalKey string) *CloudflareClient {
	return NewCloudflareClientWithBaseURLAndAuth(baseURL, CloudflareAuth{
		Type:  authTypeGlobalKey,
		Email: email,
		Key:   globalKey,
	})
}

// NewCloudflareClientWithAuth 创建使用 Global API Key 的客户端。
func NewCloudflareClientWithAuth(auth CloudflareAuth) *CloudflareClient {
	return NewCloudflareClientWithBaseURLAndAuth(cloudflareAPIBaseURL, auth)
}

// NewCloudflareClientWithBaseURLAndAuth 创建可指定 API 地址和 Global API Key 的客户端。
func NewCloudflareClientWithBaseURLAndAuth(baseURL string, auth CloudflareAuth) *CloudflareClient {
	auth.Type = NormalizeAuthType(auth.Type)
	auth.Email = strings.TrimSpace(auth.Email)
	auth.Key = strings.TrimSpace(auth.Key)
	return &CloudflareClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		auth:    auth,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// CreateTunnel 创建远程管理 Tunnel，并尽量获取首次运行所需的 Tunnel Token。
func (c *CloudflareClient) CreateTunnel(ctx context.Context, accountID string, name string) (CloudflareTunnel, error) {
	body := map[string]any{
		"name":       strings.TrimSpace(name),
		"config_src": "cloudflare",
	}
	var tunnel CloudflareTunnel
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/accounts/%s/cfd_tunnel", accountID), nil, body, &tunnel); err != nil {
		return tunnel, err
	}
	if tunnel.Token == "" && tunnel.ID != "" {
		token, err := c.GetTunnelToken(ctx, accountID, tunnel.ID)
		if err == nil {
			tunnel.Token = token
		}
	}
	return tunnel, nil
}

// ListTunnels 列出账号下已有的 Cloudflare Tunnels。
func (c *CloudflareClient) ListTunnels(ctx context.Context, accountID string) ([]CloudflareTunnel, error) {
	tunnels := []CloudflareTunnel{}
	page := 1
	for {
		query := url.Values{}
		query.Set("per_page", "50")
		query.Set("page", fmt.Sprintf("%d", page))
		envelope, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/accounts/%s/cfd_tunnel", accountID), query, nil)
		if err != nil {
			return nil, err
		}
		var current []CloudflareTunnel
		if err := json.Unmarshal(envelope.Result, &current); err != nil {
			return nil, fmt.Errorf("解析 Cloudflare Tunnel 列表失败: %w", err)
		}
		tunnels = append(tunnels, current...)
		if envelope.ResultInfo.TotalPages <= page || envelope.ResultInfo.TotalPages == 0 || len(current) == 0 {
			break
		}
		page++
	}
	return tunnels, nil
}

// DeleteTunnel 删除账号下指定 Tunnel。
func (c *CloudflareClient) DeleteTunnel(ctx context.Context, accountID string, tunnelID string) error {
	var ignored map[string]any
	return c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/accounts/%s/cfd_tunnel/%s", accountID, tunnelID), nil, nil, &ignored)
}

// GetTunnel 读取 Tunnel 当前状态。
func (c *CloudflareClient) GetTunnel(ctx context.Context, accountID string, tunnelID string) (CloudflareTunnel, error) {
	var tunnel CloudflareTunnel
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/accounts/%s/cfd_tunnel/%s", accountID, tunnelID), nil, nil, &tunnel)
	return tunnel, err
}

// ListAccounts 列出当前凭据可访问的 Cloudflare 账号。
func (c *CloudflareClient) ListAccounts(ctx context.Context) ([]CloudflareAccount, error) {
	accounts := []CloudflareAccount{}
	page := 1
	for {
		query := url.Values{}
		query.Set("per_page", "50")
		query.Set("page", fmt.Sprintf("%d", page))
		envelope, err := c.do(ctx, http.MethodGet, "/accounts", query, nil)
		if err != nil {
			return nil, err
		}
		var current []CloudflareAccount
		if err := json.Unmarshal(envelope.Result, &current); err != nil {
			return nil, fmt.Errorf("解析 Cloudflare Accounts 失败: %w", err)
		}
		accounts = append(accounts, current...)
		if envelope.ResultInfo.TotalPages <= page || envelope.ResultInfo.TotalPages == 0 || len(current) == 0 {
			break
		}
		page++
	}
	return accounts, nil
}

// GetZone 根据 Zone ID 获取根域名信息。
func (c *CloudflareClient) GetZone(ctx context.Context, zoneID string) (CloudflareZone, error) {
	var zone CloudflareZone
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/zones/%s", zoneID), nil, nil, &zone)
	return zone, err
}

// ListZones 列出当前 Token 可访问的域名，传入 Account ID 时只查该账号。
func (c *CloudflareClient) ListZones(ctx context.Context, accountID string) ([]CloudflareZone, error) {
	zones := []CloudflareZone{}
	page := 1
	for {
		query := url.Values{}
		query.Set("per_page", "50")
		query.Set("page", fmt.Sprintf("%d", page))
		if strings.TrimSpace(accountID) != "" {
			query.Set("account.id", strings.TrimSpace(accountID))
		}
		envelope, err := c.do(ctx, http.MethodGet, "/zones", query, nil)
		if err != nil {
			return nil, err
		}
		var current []CloudflareZone
		if err := json.Unmarshal(envelope.Result, &current); err != nil {
			return nil, fmt.Errorf("解析 Cloudflare Zones 失败: %w", err)
		}
		zones = append(zones, current...)
		if envelope.ResultInfo.TotalPages <= page || envelope.ResultInfo.TotalPages == 0 || len(current) == 0 {
			break
		}
		page++
	}
	return zones, nil
}

// GetTunnelToken 获取远程管理 Tunnel 的运行 token。
func (c *CloudflareClient) GetTunnelToken(ctx context.Context, accountID string, tunnelID string) (string, error) {
	envelope, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/token", accountID, tunnelID), nil, nil)
	if err != nil {
		return "", err
	}
	var token string
	if err := json.Unmarshal(envelope.Result, &token); err == nil && token != "" {
		return token, nil
	}
	var object struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(envelope.Result, &object); err == nil && object.Token != "" {
		return object.Token, nil
	}
	return "", fmt.Errorf("Cloudflare 未返回 Tunnel Token")
}

// PutTunnelConfiguration 将当前路由写入远程 Tunnel 配置。
func (c *CloudflareClient) PutTunnelConfiguration(ctx context.Context, accountID string, tunnelID string, routes []Route) error {
	ingress := make([]tunnelIngressRule, 0, len(routes)+1)
	for _, route := range routes {
		if !route.Enabled {
			continue
		}
		ingress = append(ingress, tunnelIngressRule{
			Hostname:      route.Hostname,
			Service:       BuildServiceURL(route),
			OriginRequest: map[string]any{},
		})
	}
	ingress = append(ingress, tunnelIngressRule{Service: "http_status:404"})
	payload := tunnelConfigurationPayload{Config: tunnelConfiguration{Ingress: ingress}}
	var ignored map[string]any
	return c.doJSON(ctx, http.MethodPut, fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/configurations", accountID, tunnelID), nil, payload, &ignored)
}

// GetTunnelConfiguration 读取远程管理 Tunnel 当前保存的 ingress 配置。
func (c *CloudflareClient) GetTunnelConfiguration(ctx context.Context, accountID string, tunnelID string) (tunnelConfiguration, error) {
	var response tunnelConfigurationResponse
	err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/accounts/%s/cfd_tunnel/%s/configurations", accountID, tunnelID), nil, nil, &response)
	return response.Config, err
}

// EnsureTunnelDNSRecord 创建或更新公开域名对应的 CNAME 记录。
func (c *CloudflareClient) EnsureTunnelDNSRecord(ctx context.Context, zoneID string, tunnelID string, hostname string) (DNSRecord, error) {
	target := tunnelDNSContent(tunnelID)
	records, err := c.ListCNAMERecords(ctx, zoneID, hostname)
	if err != nil {
		return DNSRecord{}, err
	}
	body := map[string]any{
		"type":    "CNAME",
		"name":    hostname,
		"content": target,
		"proxied": true,
		"ttl":     1,
	}
	if len(records) > 0 {
		var updated DNSRecord
		err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, records[0].ID), nil, body, &updated)
		return updated, err
	}
	var created DNSRecord
	err = c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/zones/%s/dns_records", zoneID), nil, body, &created)
	return created, err
}

// DeleteTunnelDNSRecord 删除本 Tunnel 管理的指定 CNAME 记录。
func (c *CloudflareClient) DeleteTunnelDNSRecord(ctx context.Context, zoneID string, tunnelID string, hostname string) (bool, error) {
	target := tunnelDNSContent(tunnelID)
	records, err := c.ListCNAMERecords(ctx, zoneID, hostname)
	if err != nil {
		return false, err
	}
	deleted := false
	for _, record := range records {
		if !strings.EqualFold(record.Content, target) {
			continue
		}
		var ignored map[string]any
		if err := c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, record.ID), nil, nil, &ignored); err != nil {
			return deleted, err
		}
		deleted = true
	}
	return deleted, nil
}

// DeleteTunnelDNSRecords 删除当前 Zone 中所有指向指定 Tunnel 的 CNAME 记录。
func (c *CloudflareClient) DeleteTunnelDNSRecords(ctx context.Context, zoneID string, tunnelID string) (int, error) {
	records, err := c.ListTunnelDNSRecords(ctx, zoneID, tunnelID)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, record := range records {
		var ignored map[string]any
		if err := c.doJSON(ctx, http.MethodDelete, fmt.Sprintf("/zones/%s/dns_records/%s", zoneID, record.ID), nil, nil, &ignored); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// ListTunnelDNSRecords 列出当前 Zone 中指向指定 Tunnel 的 CNAME 记录。
func (c *CloudflareClient) ListTunnelDNSRecords(ctx context.Context, zoneID string, tunnelID string) ([]DNSRecord, error) {
	target := tunnelDNSContent(tunnelID)
	records, err := c.ListZoneCNAMERecords(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	filtered := []DNSRecord{}
	for _, record := range records {
		if strings.EqualFold(record.Content, target) {
			filtered = append(filtered, record)
		}
	}
	return filtered, nil
}

// ListZoneCNAMERecords 列出当前 Zone 中所有 CNAME 记录。
func (c *CloudflareClient) ListZoneCNAMERecords(ctx context.Context, zoneID string) ([]DNSRecord, error) {
	records := []DNSRecord{}
	page := 1
	for {
		query := url.Values{}
		query.Set("type", "CNAME")
		query.Set("per_page", "100")
		query.Set("page", fmt.Sprintf("%d", page))
		envelope, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/zones/%s/dns_records", zoneID), query, nil)
		if err != nil {
			return nil, err
		}
		var current []DNSRecord
		if err := json.Unmarshal(envelope.Result, &current); err != nil {
			return nil, fmt.Errorf("解析 Cloudflare DNS 记录失败: %w", err)
		}
		records = append(records, current...)
		if envelope.ResultInfo.TotalPages <= page || envelope.ResultInfo.TotalPages == 0 || len(current) == 0 {
			break
		}
		page++
	}
	return records, nil
}

// ListCNAMERecords 查询指定 hostname 的 CNAME 记录。
func (c *CloudflareClient) ListCNAMERecords(ctx context.Context, zoneID string, hostname string) ([]DNSRecord, error) {
	query := url.Values{}
	query.Set("type", "CNAME")
	query.Set("name", hostname)
	var records []DNSRecord
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/zones/%s/dns_records", zoneID), query, nil, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// tunnelDNSContent 生成 Cloudflare Tunnel 的 CNAME 目标值。
func tunnelDNSContent(tunnelID string) string {
	return tunnelID + ".cfargotunnel.com"
}

// doJSON 执行请求并把 result 字段解析为目标类型。
func (c *CloudflareClient) doJSON(ctx context.Context, method string, path string, query url.Values, body any, target any) error {
	envelope, err := c.do(ctx, method, path, query, body)
	if err != nil {
		return err
	}
	if target == nil || len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, target); err != nil {
		return fmt.Errorf("解析 Cloudflare 响应失败: %w", err)
	}
	return nil
}

// do 执行底层 HTTP 请求并统一处理 Cloudflare 错误响应。
func (c *CloudflareClient) do(ctx context.Context, method string, path string, query url.Values, body any) (cloudflareAPIEnvelope, error) {
	if c.auth.Email == "" || c.auth.Key == "" {
		return cloudflareAPIEnvelope{}, fmt.Errorf("请先输入 Cloudflare 邮箱和 Global API Key")
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return cloudflareAPIEnvelope{}, fmt.Errorf("序列化 Cloudflare 请求失败: %w", err)
		}
		reader = bytes.NewReader(data)
	}
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return cloudflareAPIEnvelope{}, fmt.Errorf("创建 Cloudflare 请求失败: %w", err)
	}
	req.Header.Set("X-Auth-Email", c.auth.Email)
	req.Header.Set("X-Auth-Key", c.auth.Key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return cloudflareAPIEnvelope{}, fmt.Errorf("Cloudflare 请求失败: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return cloudflareAPIEnvelope{}, fmt.Errorf("读取 Cloudflare 响应失败: %w", err)
	}
	var envelope cloudflareAPIEnvelope
	if len(data) > 0 {
		if err := json.Unmarshal(data, &envelope); err != nil {
			return cloudflareAPIEnvelope{}, fmt.Errorf("Cloudflare 返回非 JSON 响应，HTTP %d", resp.StatusCode)
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !envelope.Success {
		return cloudflareAPIEnvelope{}, fmt.Errorf("Cloudflare API 错误: %s", formatCloudflareErrors(resp.StatusCode, envelope.Errors))
	}
	return envelope, nil
}

// formatCloudflareErrors 将 API 错误压缩为适合 UI 展示的短文本。
func formatCloudflareErrors(statusCode int, messages []cloudflareAPIMessage) string {
	if len(messages) == 0 {
		return fmt.Sprintf("HTTP %d", statusCode)
	}
	parts := make([]string, 0, len(messages))
	for _, item := range messages {
		if item.Code > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", item.Code, item.Message))
			continue
		}
		parts = append(parts, item.Message)
	}
	return strings.Join(parts, "; ")
}
