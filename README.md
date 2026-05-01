# Cloudflare Tunnel Desktop

一个基于 Go + Wails + React/TypeScript 的 macOS 桌面工具，用图形界面管理单个 Cloudflare Tunnel 下的多个 HTTP/HTTPS 域名映射。

## 当前能力

- 手动配置 Account ID、Zone ID、根域名、Tunnel ID/名称。
- API Token 和 Tunnel Token 按自用场景明文保存到本地配置文件，界面可查看和复制。
- 创建远程管理 Tunnel，获取 Tunnel Token。
- 管理 `hostname -> http(s)://host:port` 映射。
- 同步 Cloudflare Tunnel ingress 配置和 CNAME DNS 记录。
- 启动、停止、重启 `cloudflared`，支持 `auto`、`quic`、`http2` 传输协议。
- 监听 cloudflared 日志，标记 1033、连接、DNS、鉴权相关事件。
- 网络变化、进程异常退出、Tunnel down/degraded 时按退避策略尝试恢复。

## Cloudflare 认证

建议创建只限制到目标 Account 和 Zone 的 API Token：

- Account 权限：Cloudflare Tunnel Read + Write/Edit。
- Zone 权限：Zone Read、DNS Read + Write/Edit。

应用也支持 Global API Key。使用 Global API Key 时必须同时填写 Cloudflare 账号邮箱，应用会改用 `X-Auth-Email` 和 `X-Auth-Key` 请求头；如果把 Global API Key 当作 API Token 使用，会得到 `9109 Invalid access token`。

应用会把认证凭据和 Tunnel Token 明文写入配置文件，macOS 通常是：

```text
~/Library/Application Support/CloudflareTunnelDesktop/config.json
```

截图里的“用户 API 令牌”是推荐使用的 API Token；下面“API 密钥”里的 Global API Key 也能用，但权限更大，使用时请选择认证方式为 `Global API Key` 并填写账号邮箱。

## 开发

首次开发需要 Wails CLI：

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

运行开发模式：

```bash
wails dev
```

如果 `wails` 不在 PATH 中，可使用：

```bash
~/go/bin/wails dev
```

运行已构建的本机二进制：

```bash
./build/bin/cloudflare-tunnel-desktop
```

这个命令会打开桌面窗口。应用运行 Tunnel 时会从 `PATH` 查找 `cloudflared`，所以正式使用前需要确认：

```bash
cloudflared --version
```

如果希望 macOS Dock 显示应用图标，而不是 `exec` 通用图标，请使用打包后的 `.app` 启动：

```bash
open build/bin/cloudflare-tunnel-desktop.app
```

应用启动后会在 macOS 顶部菜单栏显示 `CF` 入口。关闭主窗口只会隐藏窗口，后台进程仍会保留；需要完全退出时，在顶部菜单栏点击 `CF`，选择 `彻底退出`。

## 换电脑安装

只运行应用时需要：

- macOS 电脑。
- `cloudflared` CLI，确保 `cloudflared --version` 可执行。
- 构建好的 `cloudflare-tunnel-desktop` 二进制或 `.app`。
- Cloudflare API Token，或 Global API Key + Cloudflare 账号邮箱。API Token 权限至少包含目标账号的 Tunnel Read/Write 和目标 Zone 的 Zone Read、DNS Read/Write。

如果要在新电脑上从源码开发或重新构建，还需要：

- Go。
- Node.js 和 npm。
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`。
- `cloudflared` CLI。

## 验证

```bash
go test ./...
cd frontend && npm run build
```

生产构建：

```bash
~/go/bin/wails build
```

如果 macOS codesign 报 `resource fork, Finder information, or similar detritus not allowed`，清理构建产物扩展属性后再验签：

```bash
xattr -cr build/bin/cloudflare-tunnel-desktop.app
xattr -d com.apple.FinderInfo build/bin/cloudflare-tunnel-desktop.app 2>/dev/null || true
xattr -d 'com.apple.fileprovider.fpfs#P' build/bin/cloudflare-tunnel-desktop.app 2>/dev/null || true
codesign --verify --deep --strict build/bin/cloudflare-tunnel-desktop.app
```

如果项目目录位于会自动注入 FileProvider 扩展属性的位置，可用 `ditto --norsrc` 重新复制后签名：

```bash
rm -rf /tmp/cloudflare-tunnel-desktop.app
ditto --norsrc build/bin/cloudflare-tunnel-desktop.app /tmp/cloudflare-tunnel-desktop.app
codesign --force --deep --sign - /tmp/cloudflare-tunnel-desktop.app
rm -rf build/bin/cloudflare-tunnel-desktop.app
ditto --norsrc /tmp/cloudflare-tunnel-desktop.app build/bin/cloudflare-tunnel-desktop.app
codesign --verify --deep --strict build/bin/cloudflare-tunnel-desktop.app
```
