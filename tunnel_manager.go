package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

var cloudflaredExecutable = cloudflaredExecutableName()

var cloudflaredCommonPaths = []string{
	"/opt/homebrew/bin/cloudflared",
	"/usr/local/bin/cloudflared",
}

// TunnelManager 管理 cloudflared 子进程和本地运行日志。
type TunnelManager struct {
	mu                 sync.Mutex
	cmd                *exec.Cmd
	cancel             context.CancelFunc
	running            bool
	desiredRunning     bool
	startedAt          time.Time
	pid                int
	protocol           string
	lastToken          string
	lastTunnelID       string
	autoRestart        bool
	restartAttempts    int
	lastError          string
	lastEvent          string
	cloudflaredPath    string
	cloudflaredVersion string
	logs               []LogEntry
	onLog              func(LogEntry)
}

// NewTunnelManager 创建 cloudflared 进程管理器。
func NewTunnelManager(onLog func(LogEntry)) *TunnelManager {
	manager := &TunnelManager{
		protocol:    defaultProtocol,
		autoRestart: true,
		logs:        []LogEntry{},
		onLog:       onLog,
	}
	manager.refreshCloudflaredInfo()
	return manager
}

// BuildCloudflaredArgs 生成不包含 token 的 cloudflared 启动参数。
func BuildCloudflaredArgs(protocol string, tunnelID string) []string {
	return []string{
		"tunnel",
		"--protocol", normalizeProtocol(protocol),
		"--no-autoupdate",
		"--loglevel", "info",
		"--output", "json",
		"run",
		tunnelID,
	}
}

// Start 启动 cloudflared，Tunnel Token 只通过环境变量传入。
func (m *TunnelManager) Start(token string, tunnelID string, protocol string, autoRestart bool) error {
	return m.startProcess(token, tunnelID, protocol, autoRestart, true)
}

// startProcess 执行实际启动逻辑，自动恢复时保留退避计数。
func (m *TunnelManager) startProcess(token string, tunnelID string, protocol string, autoRestart bool, resetAttempts bool) error {
	token = strings.TrimSpace(token)
	tunnelID = strings.TrimSpace(tunnelID)
	protocol = normalizeProtocol(protocol)
	if token == "" {
		return fmt.Errorf("请先输入或获取 Tunnel Token")
	}
	if !uuidPattern.MatchString(tunnelID) {
		return fmt.Errorf("Tunnel ID 必须是 UUID 格式")
	}
	if !isValidProtocol(protocol) {
		return fmt.Errorf("传输协议只支持 auto、quic 或 http2")
	}
	path, err := findCloudflaredPath()
	if err != nil {
		return fmt.Errorf("未找到 cloudflared，请先安装 cloudflared CLI")
	}

	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("cloudflared 已经在运行")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, path, BuildCloudflaredArgs(protocol, tunnelID)...)
	configureBackgroundCommand(cmd)
	cmd.Env = append(os.Environ(), "TUNNEL_TOKEN="+token)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		m.mu.Unlock()
		return fmt.Errorf("连接 cloudflared 标准输出失败: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		m.mu.Unlock()
		return fmt.Errorf("连接 cloudflared 错误输出失败: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		m.mu.Unlock()
		return fmt.Errorf("启动 cloudflared 失败: %w", err)
	}
	m.cmd = cmd
	m.cancel = cancel
	m.running = true
	m.desiredRunning = true
	m.startedAt = time.Now()
	m.pid = cmd.Process.Pid
	m.protocol = protocol
	m.lastToken = token
	m.lastTunnelID = tunnelID
	m.autoRestart = autoRestart
	if resetAttempts {
		m.restartAttempts = 0
	}
	m.lastError = ""
	m.lastEvent = "cloudflared 已启动"
	m.cloudflaredPath = path
	m.mu.Unlock()

	go m.scanPipe(stdout, "stdout")
	go m.scanPipe(stderr, "stderr")
	go m.waitForExit(cmd)
	m.addLog("info", "cloudflared", fmt.Sprintf("cloudflared 已启动，PID %d，协议 %s", cmd.Process.Pid, protocol), "process")
	return nil
}

// Stop 停止 cloudflared，并关闭自动重启意图。
func (m *TunnelManager) Stop() error {
	m.mu.Lock()
	if !m.running || m.cmd == nil || m.cmd.Process == nil {
		m.desiredRunning = false
		m.mu.Unlock()
		return nil
	}
	cmd := m.cmd
	m.desiredRunning = false
	m.lastEvent = "正在停止 cloudflared"
	m.mu.Unlock()

	m.addLog("info", "cloudflared", "正在停止 cloudflared", "process")
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("发送停止信号失败: %w", err)
	}
	go m.killIfStillRunning(cmd, 5*time.Second)
	return nil
}

// Restart 按当前或指定参数重启 cloudflared。
func (m *TunnelManager) Restart(token string, tunnelID string, protocol string, autoRestart bool) error {
	if err := m.Stop(); err != nil {
		return err
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning() {
			break
		}
		time.Sleep(120 * time.Millisecond)
	}
	return m.Start(token, tunnelID, protocol, autoRestart)
}

// IsRunning 返回 cloudflared 当前是否仍在运行。
func (m *TunnelManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Status 返回进程管理器维护的运行状态。
func (m *TunnelManager) Status() RuntimeStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	uptime := int64(0)
	if m.running && !m.startedAt.IsZero() {
		uptime = int64(time.Since(m.startedAt).Seconds())
	}
	return RuntimeStatus{
		CloudflaredPath:    m.cloudflaredPath,
		CloudflaredVersion: m.cloudflaredVersion,
		Running:            m.running,
		PID:                m.pid,
		Protocol:           m.protocol,
		UptimeSeconds:      uptime,
		LastError:          m.lastError,
		LastEvent:          m.lastEvent,
		AutoRestart:        m.autoRestart,
		RestartAttempts:    m.restartAttempts,
	}
}

// Logs 返回最近的运行日志副本。
func (m *TunnelManager) Logs() []LogEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]LogEntry, len(m.logs))
	copy(copied, m.logs)
	return copied
}

// SetAutoRestart 更新自动重启开关。
func (m *TunnelManager) SetAutoRestart(enabled bool) {
	m.mu.Lock()
	m.autoRestart = enabled
	m.mu.Unlock()
}

// TriggerRecovery 在网络变化或健康检查失败时触发恢复性重启。
func (m *TunnelManager) TriggerRecovery(reason string) {
	m.mu.Lock()
	if !m.running || !m.autoRestart || m.lastToken == "" || m.lastTunnelID == "" {
		m.mu.Unlock()
		return
	}
	token := m.lastToken
	tunnelID := m.lastTunnelID
	protocol := m.protocol
	autoRestart := m.autoRestart
	m.mu.Unlock()

	m.addLog("warn", "health", "检测到"+reason+"，准备重启 cloudflared", "recovery")
	go func() {
		if err := m.Restart(token, tunnelID, protocol, autoRestart); err != nil {
			m.addLog("error", "health", "恢复性重启失败: "+err.Error(), "recovery")
		}
	}()
}

// scanPipe 持续读取 cloudflared 输出并写入 UI 日志。
func (m *TunnelManager) scanPipe(pipe any, source string) {
	reader, ok := pipe.(interface {
		Read([]byte) (int, error)
	})
	if !ok {
		return
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		level, message, category := parseCloudflaredLine(scanner.Text())
		m.addLog(level, source, message, category)
	}
	if err := scanner.Err(); err != nil {
		m.addLog("warn", source, "读取 cloudflared 日志失败: "+err.Error(), "log")
	}
}

// waitForExit 监听子进程退出并在需要时执行退避重启。
func (m *TunnelManager) waitForExit(cmd *exec.Cmd) {
	err := cmd.Wait()

	m.mu.Lock()
	if m.cmd != cmd {
		m.mu.Unlock()
		return
	}
	desired := m.desiredRunning
	autoRestart := m.autoRestart
	token := m.lastToken
	tunnelID := m.lastTunnelID
	protocol := m.protocol
	m.running = false
	m.pid = 0
	m.cmd = nil
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if err != nil && desired {
		m.lastError = RedactSensitiveText(err.Error())
	}
	m.lastEvent = "cloudflared 已退出"
	if desired && autoRestart {
		m.restartAttempts++
	}
	attempt := m.restartAttempts
	m.mu.Unlock()

	if err != nil {
		m.addLog("warn", "cloudflared", "cloudflared 已退出: "+RedactSensitiveText(err.Error()), "process")
	} else {
		m.addLog("info", "cloudflared", "cloudflared 已退出", "process")
	}
	if desired && autoRestart && token != "" && tunnelID != "" {
		delay := restartDelay(attempt)
		m.addLog("warn", "health", fmt.Sprintf("%s 后自动重启 cloudflared", delay), "recovery")
		time.Sleep(delay)
		if err := m.startProcess(token, tunnelID, protocol, autoRestart, false); err != nil {
			m.addLog("error", "health", "自动重启失败: "+err.Error(), "recovery")
		}
	}
}

// killIfStillRunning 在优雅停止超时后强制结束同一个进程。
func (m *TunnelManager) killIfStillRunning(cmd *exec.Cmd, delay time.Duration) {
	time.Sleep(delay)
	m.mu.Lock()
	stillRunning := m.cmd == cmd && m.running && cmd.Process != nil
	m.mu.Unlock()
	if stillRunning {
		_ = cmd.Process.Kill()
		m.addLog("warn", "cloudflared", "cloudflared 停止超时，已强制结束", "process")
	}
}

// addLog 记录日志并通知 Wails 前端。
func (m *TunnelManager) addLog(level string, source string, message string, category string) {
	entry := LogEntry{
		Time:     time.Now(),
		Level:    level,
		Source:   source,
		Message:  RedactSensitiveText(message),
		Category: category,
	}
	m.mu.Lock()
	if level == "error" {
		m.lastError = entry.Message
	}
	m.lastEvent = entry.Message
	m.logs = append(m.logs, entry)
	if len(m.logs) > maxLogEntries {
		m.logs = m.logs[len(m.logs)-maxLogEntries:]
	}
	callback := m.onLog
	m.mu.Unlock()
	if callback != nil {
		callback(entry)
	}
}

// refreshCloudflaredInfo 读取 cloudflared 路径和版本，失败时只记录状态不阻断应用启动。
func (m *TunnelManager) refreshCloudflaredInfo() {
	path, err := findCloudflaredPath()
	if err != nil {
		m.cloudflaredPath = ""
		m.cloudflaredVersion = "未安装"
		return
	}
	cmd := exec.Command(path, "--version")
	configureBackgroundCommand(cmd)
	output, err := cmd.CombinedOutput()
	m.cloudflaredPath = path
	if err != nil {
		m.cloudflaredVersion = "版本读取失败"
		return
	}
	m.cloudflaredVersion = strings.TrimSpace(string(output))
}

// findCloudflaredPath 查找 cloudflared，可兼容桌面应用启动时 PATH 不完整的情况。
func findCloudflaredPath() (string, error) {
	if path, err := exec.LookPath(cloudflaredExecutable); err == nil {
		return path, nil
	}
	for _, path := range cloudflaredCandidatePaths() {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("未找到 cloudflared")
}

// cloudflaredExecutableName 返回当前系统默认的 cloudflared 可执行文件名。
func cloudflaredExecutableName() string {
	if runtime.GOOS == "windows" {
		return "cloudflared.exe"
	}
	return "cloudflared"
}

// cloudflaredCandidatePaths 返回桌面启动场景中 PATH 之外的常见安装位置。
func cloudflaredCandidatePaths() []string {
	paths := append([]string{}, cloudflaredCommonPaths...)
	if runtime.GOOS != "windows" {
		return paths
	}
	paths = append(paths, windowsCloudflaredCandidatePaths()...)
	return paths
}

// windowsCloudflaredCandidatePaths 补充 winget、scoop 和 Chocolatey 的常见 cloudflared.exe 位置。
func windowsCloudflaredCandidatePaths() []string {
	paths := []string{}
	if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
		paths = append(paths, filepath.Join(programFiles, "cloudflared", "cloudflared.exe"))
	}
	if programFilesX86 := os.Getenv("ProgramFiles(x86)"); programFilesX86 != "" {
		paths = append(paths, filepath.Join(programFilesX86, "cloudflared", "cloudflared.exe"))
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		matches, _ := filepath.Glob(filepath.Join(localAppData, "Microsoft", "WinGet", "Packages", "Cloudflare.cloudflared_*", "cloudflared.exe"))
		paths = append(paths, matches...)
	}
	if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
		paths = append(paths, filepath.Join(userProfile, "scoop", "shims", "cloudflared.exe"))
	}
	if programData := os.Getenv("ProgramData"); programData != "" {
		paths = append(paths,
			filepath.Join(programData, "scoop", "shims", "cloudflared.exe"),
			filepath.Join(programData, "chocolatey", "bin", "cloudflared.exe"),
		)
	}
	return paths
}

// parseCloudflaredLine 解析 cloudflared JSON 或普通文本日志。
func parseCloudflaredLine(line string) (string, string, string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "info", "", "log"
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(line), &payload); err == nil {
		level := stringFromMap(payload, "level", "info")
		message := firstStringFromMap(payload, []string{"message", "msg", "error", "event"})
		if message == "" {
			message = line
		}
		return normalizeLogLevel(level), RedactSensitiveText(message), classifyLog(message)
	}
	return "info", RedactSensitiveText(line), classifyLog(line)
}

// firstStringFromMap 返回 map 中第一个存在的字符串字段。
func firstStringFromMap(payload map[string]any, keys []string) string {
	for _, key := range keys {
		if value := stringFromMap(payload, key, ""); value != "" {
			return value
		}
	}
	return ""
}

// stringFromMap 将 JSON map 中的字符串值安全取出。
func stringFromMap(payload map[string]any, key string, fallback string) string {
	value, ok := payload[key]
	if !ok {
		return fallback
	}
	text, ok := value.(string)
	if !ok {
		return fallback
	}
	return text
}

// normalizeLogLevel 将 cloudflared 日志级别映射到 UI 使用的固定值。
func normalizeLogLevel(level string) string {
	switch strings.ToLower(level) {
	case "debug", "info", "warn", "error":
		return strings.ToLower(level)
	case "warning":
		return "warn"
	default:
		return "info"
	}
}

// classifyLog 根据日志内容标记常见 Tunnel 故障类别。
func classifyLog(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "1033"):
		return "error1033"
	case strings.Contains(lower, "connection") || strings.Contains(lower, "reconnect") || strings.Contains(lower, "disconnected"):
		return "connection"
	case strings.Contains(lower, "dns"):
		return "dns"
	case strings.Contains(lower, "token") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "forbidden"):
		return "auth"
	default:
		return "log"
	}
}

// restartDelay 计算自动重启退避时间，避免网络切换时高频拉起进程。
func restartDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	seconds := 1 << minInt(attempt-1, 5)
	return time.Duration(seconds) * time.Second
}

// minInt 返回两个整数中的较小值。
func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
