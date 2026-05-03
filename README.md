# Cloudflare Tunnel Desktop

一个基于 Go + Wails + React/TypeScript 的桌面工具，用图形界面管理 Cloudflare Tunnel 和 HTTP/HTTPS 域名映射。

当前适配 macOS 和 Windows，GitHub Actions 已支持自动构建和发布 macOS arm64 `dmg` 与 Windows amd64 `exe`。macOS 顶部菜单栏和 Windows 通知区均支持关闭窗口后常驻，并可从托盘重新唤起或彻底退出。

## 当前能力

- 页面分为基础配置、Tunnel 管理、域名映射、日志四个工作区。
- 手动配置 Account ID、Zone ID、根域名、认证方式和运行参数。
- API Token 或 Global API Key 按自用场景明文保存到本地配置文件，界面可查看和复制。
- 创建、刷新、删除 Cloudflare Tunnel，并可将指定 Tunnel 设为当前管理对象；本地连接运行中切换 Tunnel 时会自动重启连接。
- 自动从 Cloudflare 获取当前 Tunnel 的 Tunnel Token；界面只读展示，不提供手工修改入口。
- 管理当前 Tunnel 的 `hostname -> http(s)://host:port` 映射，新增、编辑、删除后自动同步 Cloudflare Tunnel ingress 配置和 CNAME DNS 记录。
- 启动、停止、重启 `cloudflared`，支持 `auto`、`quic`、`http2` 传输协议。
- 自动检测和安装 `cloudflared`：macOS 使用 Homebrew，Windows 优先使用 `winget`，并回退到 `scoop` 或 Chocolatey。
- 监听 cloudflared 日志，标记 1033、连接、DNS、鉴权相关事件。
- 网络变化、进程异常退出、Tunnel down/degraded 时按退避策略尝试恢复。

## Cloudflare 认证

建议创建只限制到目标 Account 和 Zone 的 API Token：

- Account 权限：Cloudflare Tunnel Read + Write/Edit。
- Zone 权限：Zone Read、DNS Read + Write/Edit。

应用也支持 Global API Key。使用 Global API Key 时必须同时填写 Cloudflare 账号邮箱，应用会改用 `X-Auth-Email` 和 `X-Auth-Key` 请求头；如果把 Global API Key 当作 API Token 使用，会得到 `9109 Invalid access token`。

应用会把认证凭据明文写入配置文件，并缓存当前 Tunnel 的运行 token。macOS 通常是：

```text
~/Library/Application Support/CloudflareTunnelDesktop/config.json
```

Windows 通常是：

```text
C:\Users\<用户名>\AppData\Roaming\CloudflareTunnelDesktop\config.json
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

应用启动后会在 macOS 顶部菜单栏显示 `CF` 常驻入口，Windows 会在通知区显示托盘入口。关闭主窗口只会隐藏窗口，后台进程仍会保留；需要完全退出时，在菜单里选择 `彻底退出`。

## 换电脑安装

只运行应用时需要：

- macOS 或 Windows 电脑。
- `cloudflared` CLI，确保 `cloudflared --version` 可执行。也可以在界面中点击安装：macOS 需要 Homebrew；Windows 优先使用 Cloudflare [官方下载文档](https://developers.cloudflare.com/tunnel/downloads/)中的 `winget install --id Cloudflare.cloudflared`，如果没有 `winget` 会尝试 `scoop install cloudflared` 或 `choco install cloudflared -y`。
- 构建好的 `cloudflare-tunnel-desktop` 二进制、`.app`、`.dmg` 或 `.exe`。
- Cloudflare API Token，或 Global API Key + Cloudflare 账号邮箱。API Token 权限至少包含目标账号的 Tunnel Read/Write 和目标 Zone 的 Zone Read、DNS Read/Write。

如果要在新电脑上从源码开发或重新构建，还需要：

- Go。
- Node.js 和 npm。
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`。
- `cloudflared` CLI。

## 验证

```bash
cd frontend && npm run build
cd ..
go test ./...
```

macOS 本机生产构建：

```bash
~/go/bin/wails build
```

Windows amd64 交叉构建：

```bash
~/go/bin/wails build -platform windows/amd64 -clean -nosyncgomod
file build/bin/cloudflare-tunnel-desktop.exe
```

macOS dmg 可在构建出 `.app` 后创建：

```bash
mkdir -p dist
hdiutil create \
  -volname "Cloudflare Tunnel Desktop" \
  -srcfolder build/bin/cloudflare-tunnel-desktop.app \
  -ov \
  -fs HFS+ \
  -format UDZO \
  dist/cloudflare-tunnel-desktop-macos-arm64.dmg
hdiutil verify dist/cloudflare-tunnel-desktop-macos-arm64.dmg
```

Windows exe 的真实运行校验需要在 Windows 实机、虚拟机或 GitHub Actions Windows runner 上执行。CI 中会验证 exe 文件头，并启动应用保持 10 秒，避免只检查到“能编译”。

## GitHub Actions 发布

仓库内置 `.github/workflows/release.yml`：

- push 到 `main` 或发起 PR 时，自动构建 macOS dmg 和 Windows exe，并运行 Go 测试。
- Windows job 会验证 exe 是 PE 可执行文件，并做启动冒烟测试。托盘交互和包管理器安装仍建议在 Windows 实机或虚拟机做一次人工验收。
- macOS job 会构建 `.app`、生成 dmg，并执行 `hdiutil verify`。
- tag 以 `v` 开头时会自动发布 GitHub Release。
- 也可以在 Actions 页面手动运行 `Build and Release`，设置 `publish=true` 并填写 `release_tag`，例如 `v1.0.0`。

发布产物包括：

- `cloudflare-tunnel-desktop-macos-arm64.dmg`
- `cloudflare-tunnel-desktop-windows-amd64.exe`
- `SHA256SUMS.txt`

最新已验证发布版本是：

```text
https://github.com/18230/cloudflare-tunnel-desktop/releases/tag/v1.0.2
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
