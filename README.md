# YdisksPanel

家庭服务统一入口面板：一个页面汇总所有内网服务，自动按访问者所在网络（内网 / 公网）选择对应地址。**不是反向代理**——面板只负责导航与鉴权，服务本身如何暴露到公网由你已有的方案（Caddy、Nginx、FRP、Tailscale…）决定。

- React + Tailwind 前端，Go 后端，SQLite 存储，单二进制 / Docker 部署
- 每个服务可配置「内网地址」与「公网地址」；只配一个则始终使用它
- ⌘K 搜索、拖拽排序、在线状态检测、亮/暗主题
- 服务图标可在线选择内置图标 / emoji / 上传图片，并自选底色；**未选图标时由服务端自动抓取网站 favicon**（缓存 7 天，仅同主机、仅图片，失败回退首字母）

## 快速开始

**Docker Compose（推荐，数据持久化到命名卷 `ydiskspanel-data`）**
```bash
docker compose up -d                      # 拉取 ghcr.io/christ9038/ydiskspanel:latest
docker compose logs ydiskspanel | grep -i token   # 查看安装令牌
```
镜像基于 `scratch`（仅静态二进制 + CA 证书，约 10 MB），支持 `linux/amd64`、`linux/arm64`、`linux/arm/v7`，以非 root（uid 65532）运行。

**单二进制**：到 [Releases](../../releases) 下载 `linux_x86_64` / `linux_arm64` / `linux_armv7` 压缩包，解压后 `./ydiskspanel`；或源码构建：`make build && ./bin/ydiskspanel`。

**发布**：推送 `vX.Y.Z` 标签（或在 Actions 页手动运行 *Release*，并选择该标签）会自动构建三种架构的二进制、多架构 Docker 镜像并创建 GitHub Release。

首次启动日志会打印一次性 **安装令牌**，打开页面 `/setup` 输入它来创建管理员（防止公网被抢注）。

## 内外网如何判定
服务端根据请求来源 IP 判断：落在 `YDP_LAN_CIDRS`（默认私网段）即为内网。若面板在反向代理之后，必须设置 `YDP_TRUSTED_PROXIES` 为代理地址，面板才会采信 `X-Forwarded-For`；否则所有人都会显示为代理的 IP。顶栏可手动强制「内网 / 公网」。

| 变量 | 默认 | 说明 |
|---|---|---|
| `YDP_LISTEN` | `:8080` | 监听地址 |
| `YDP_DATA_DIR` | `./data` | `ydiskspanel.db`（含上传的图标/壁纸）、`secret.key`、网站图标缓存 |
| `YDP_LAN_CIDRS` | 私网段 | 视为内网的网段 |
| `YDP_TRUSTED_PROXIES` | 空 | 可信代理（逗号分隔 CIDR/IP） |
| `YDP_TLS_CERT` / `YDP_TLS_KEY` | 空 | 直接提供 HTTPS |
| `YDP_COOKIE_SECURE` | `auto` | `auto` / `always` / `never` |
| `YDP_HEALTH_INTERVAL` | `60s` | 状态检测间隔，`0` 关闭 |

## 安全设计
- argon2id 密码哈希；会话令牌仅存 SHA-256；HttpOnly + SameSite=Lax Cookie（HTTPS 下 Secure）
- 登录按 IP / 用户名 / 组合三重限流与指数退避；不泄露用户是否存在
- 可选 TOTP 两步验证（防重放、恢复码、密钥 AES-GCM 加密落盘），管理员可强制全员启用
- CSRF：自定义头 + Origin / Sec-Fetch-Site 校验；严格 CSP、`X-Frame-Options: DENY`、`Referrer-Policy: no-referrer`
- 服务可见性在后端按团队过滤；服务 URL 仅允许 http(s)；上传图片入库前一律解码、等比缩小（图标 ≤256px，壁纸 ≤1920px）并重新编码：去除 EXIF/GPS，禁止 SVG，拒绝像素炸弹；图片存放在 SQLite 中，未被引用的会自动清理
- 公网使用请务必走 HTTPS（前置代理或 `YDP_TLS_*`）

## 开发
```bash
go run ./cmd/ydiskspanel            # 后端 :8080
cd web && npm run dev          # 前端热更新，/api 代理到后端
make test
```

## 许可证
[MIT](LICENSE)
