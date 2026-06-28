# MCQQ Bridge

MCQQ Bridge 是一个面向 Minecraft Bedrock Dedicated Server 服主的 QQ 群互通插件 MVP。

它的目标是：

- 不要求服主安装 Docker、Redis、PostgreSQL。
- Bridge 使用 Go 编译成单文件程序。
- QQ 侧适配 OneBot v11，优先支持 NapCat / Lagrange.OneBot。
- MC 侧通过 BDS 行为包 Script API 使用 HTTP 与 Bridge 通信。
- QQ 到 MC 使用轮询队列，避免让 BDS 暴露额外端口。

## 当前已实现

- Windows 发行包可内置 NapCat Windows OneKey 启动包。
- 启动时由 `mcqq-bridge.exe init/start` 自动写入 NapCat OneBot HTTP / WebSocket 配置。
- `data/config.yml` 自动生成与保存。
- HTTP API：
  - `POST /api/mc/events`
  - `GET /api/mc/pull`
  - `POST /api/mc/ack`
  - `POST /api/mc/heartbeat`
  - `POST /onebot/event`
  - `GET /api/status`
  - `GET /api/pack/download`
- OneBot 正向 WebSocket 收群消息。
- OneBot HTTP 或 WebSocket 发送群消息。
- SQLite 消息队列、trace 去重、心跳记录。
- Web 配置页、状态页、行为包下载页、诊断页。
- 动态生成 `.mcpack` 行为包。
- `doctor` 本地诊断命令。
- Windows / Linux 启动与构建脚本。

## 开发运行

```powershell
go mod tidy
go run ./cmd/mcqq-bridge start
```

打开：

```text
http://127.0.0.1:8080/setup
```

## Windows 使用

源码版可双击：

```text
start.bat
```

`start.bat` 会先打开 NapCat 窗口，再启动 Bridge。首次启动 NapCat 时仍需要你按提示扫码/登录 QQ，这是 QQ 账号登录步骤，不能由程序代替。

源码仓库不提交 NapCat、QQ 运行时、登录缓存或构建产物。要生成面向服主的一键包，请运行：

```powershell
.\scripts\package-windows.ps1
```

生成文件：

```text
dist/MCQQ-Bridge-Windows-x64.zip
```

这个 zip 会包含 `mcqq-bridge.exe`、纯 BAT 启动脚本、干净的 `data/` / `logs/` 目录，以及从 NapCat 官方 release 下载的 Windows OneKey 启动包。

发布版推荐目录：

```text
mcqq-bridge/
├─ mcqq-bridge.exe
├─ napcat/
├─ start.bat
├─ stop.bat
├─ update.bat
├─ data/
├─ logs/
└─ README.md
```

## Linux Docker Compose 一键部署

```bash
cd deploy/compose
cp .env.example .env
vim .env
./init.sh
```

至少修改：

```env
BRIDGE_PUBLIC_URL=http://YOUR_SERVER_IP:8080
QQ_GROUP_ID=123456789
```

`init.sh` 会构建并启动两个服务：

```text
bridge  MCQQ Bridge、Web 配置页、行为包生成器
napcat  QQ 登录和 OneBot v11 服务
```

常用入口：

```bash
./logs.sh
./logs.sh bridge
./logs.sh napcat
./doctor.sh
./generate-pack.sh
```

打开：

```text
Bridge Web UI: http://YOUR_SERVER_IP:8080/setup
NapCat WebUI:  http://YOUR_SERVER_IP:6099/webui
```

行为包会生成到：

```text
deploy/compose/packs/mcqq-bridge-behavior-pack.mcpack
```

把这个行为包安装到 BDS 世界并启用即可。Linux 详细说明见：

```text
deploy/compose/README.md
```

### Linux 无浏览器配置

Docker Compose 版即使没有桌面浏览器，也可以在 SSH 里直接改配置：

```bash
cd deploy/compose
docker compose exec bridge config show
docker compose exec bridge config set qq.group_id 123456789
docker compose exec bridge config set server.public_url http://YOUR_SERVER_IP:8080
docker compose exec bridge config set qq.forward_prefix ""
docker compose exec bridge pack generate /app/packs/mcqq-bridge-behavior-pack.mcpack
docker compose restart bridge
```

常用配置项：

```text
server.public_url              BDS 能访问到的 Bridge 地址
minecraft.server_id            MC 服务器 ID，默认 survival
qq.group_id                    目标 QQ 群号
qq.forward_prefix              QQ 转 MC 前缀，默认空；设置成 /mc 可降低打扰
onebot.ws_url                  OneBot WebSocket 地址
onebot.http_url                OneBot HTTP 地址
onebot.access_token            OneBot token
features.mc_to_qq_chat         true/false
features.qq_to_mc_chat         true/false
```

查看完整配置：

```bash
./mcqq-bridge config show
```

生成出的 `mcqq-bridge-behavior-pack.mcpack` 可以用 `scp`、服务器面板文件管理器，或在本地解压后复制到 BDS 世界的 `behavior_packs` 目录并启用。

## 构建发行文件

仅构建二进制：

```powershell
.\scripts\build.ps1
```

构建 Windows 一键发行包：

```powershell
.\scripts\package-windows.ps1
```

Linux/macOS 交叉构建：

```bash
bash ./scripts/build.sh
```

## NapCat / Lagrange 配置

Bridge 默认配置：

```yaml
onebot:
  ws_url: "ws://127.0.0.1:3001"
  http_url: "http://127.0.0.1:3000"
  access_token: "auto-generated-token"
```

本发行包会在 `mcqq-bridge.exe init/start` 时自动读取 `data/config.yml` 里的 `onebot.access_token`，并写入 NapCat 的 `onebot11.json`。启动入口仍然是纯 `start.bat`，不依赖 PowerShell。

默认启用：

```text
OneBot HTTP Server: http://127.0.0.1:3000
OneBot WebSocket Server: ws://127.0.0.1:3001
```

如果你已经有自己的 NapCat / Lagrange，只需要在 `/setup` 里把 OneBot 地址改成你的现有地址即可。

### 首次登录流程

1. 双击 `start.bat`。
2. 等待 NapCat 窗口出现。
3. 按 NapCat/QQ 提示扫码登录。
4. Bridge 会打开 `http://127.0.0.1:8080/setup`。
5. 填 QQ 群号，点击“发送 QQ 测试消息”。
6. 打开 `/pack` 下载行为包并安装到 BDS。

## 行为包安装

1. 启动 Bridge。
2. 打开 `/setup` 配置 Bridge 访问地址、QQ群号、OneBot 地址。
3. 打开 `/pack` 下载 `mcqq-bridge-behavior-pack.mcpack`。
4. 将行为包安装到 BDS 世界并启用。
5. 确保 BDS 版本支持 `@minecraft/server-net`，并已开启脚本相关实验能力。

## 消息规则

- MC 聊天会转发到 QQ：`[MC] Steve：hello`
- QQ 群消息默认会直接进入游戏：`hello`
- 如果想降低群聊打扰，可以在配置页把“QQ 转 MC 前缀”设置成 `/mc`
- MC 行为包每 40 ticks 拉取一次 QQ 消息。

## 诊断

```powershell
go run ./cmd/mcqq-bridge doctor
```

发布版：

```bash
./mcqq-bridge doctor
```
