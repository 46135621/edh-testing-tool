# Commander Power Check

输入公开的 Moxfield Commander 牌组 URL 或粘贴牌表文本，同时查询 CommanderSalt 与 EDH Power Level 的评分，并给出构筑缺口推荐与轻量组牌工具。

## 给客户：直接下载客户端

客户只需要 GitHub Releases 页面里的一个文件：

- **`edh-powerlevel-client.exe`** —— 双击即用。它是 Tauri 外壳，内部已嵌入 Go 服务端与前端，首次启动会把自己释放到 `%LOCALAPPDATA%\EDHPowerLevel\runtime\` 并随机挑一个空闲端口运行，然后自动打开窗口。

不需要安装 Go、Rust、Node 或单独跑 `server.exe`。Releases 由推送 `main` 分支自动触发构建（版本号取最后一次提交的时间）。

### 使用前提

1. **联网且能访问海外服务**。评分依赖 Scryfall、CommanderSalt、EDH Power Level、EDHREC、Commander Spellbook、Moxfield 等外部站点，网络不通时相关评分会显示「跳过」。
2. **本机有 Chrome / Edge**。EDH Power Level 的评分复用本机浏览器内核抓取页面。
3. **Windows 系统建议带 WebView2 Runtime**。Tauri 窗口渲染依赖它；Win10/11 多数自带，精简系统或老系统需另行安装。

## 给开发者：本地运行

需要 Go 1.24+。首次会自动联网下载依赖，并自动查找本机的 Chrome / Chromium / Edge 做 EDH Power Level 评分。

```bash
go run ./cmd/server
```

打开 <http://localhost:18781>。

其他平台手动指定浏览器路径：

```bash
BROWSER_PATH=/usr/bin/chromium go run ./cmd/server
```

开发时可用重启脚本：先构建新版本，构建成功后只终止由该脚本记录并验证过的旧进程，再启动新进程并等待健康检查通过，不会按端口或进程名误杀其他程序。

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\restart.ps1
```

进程状态记录在 `.run/powerlevel.pid`，日志在 `.run/powerlevel.log` 与 `.run/powerlevel.error.log`。直接用 `go run` 启动的旧进程不受脚本管理，第一次需手动停掉，之后统一用重启脚本即可。

## 构建桌面客户端

单文件客户端通过 Tauri 构建，内置服务端二进制由 `include_bytes!` 编译进 exe：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-client.ps1
```

需要本机装有 Rust（`cargo`）与 Go。产物在 `src-tauri\target\release\edh-powerlevel-client.exe`。CI 的 `.github/workflows/release.yml` 会自动完成同样的构建并发布到 Releases，正常交付不必在本地手动构建。

## 配置

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `APP_ADDRESS` | `:18781` | 监听地址 |
| `REQUEST_TIMEOUT` | `90s` | 整体请求超时 |
| `PROVIDER_TIMEOUT` | `60s` | 单个第三方分析超时 |
| `CACHE_TTL` | `30m` | 完整分析结果缓存时间 |
| `PARTIAL_CACHE_TTL` | `45s` | 第三方部分失败结果的短缓存时间 |
| `CACHE_MAX_ENTRIES` | `500` | TTL/LRU 结果缓存容量 |
| `EDH_MAX_CONCURRENCY` | `2` | 同时执行的 EDH 浏览器分析上限 |
| `COMMANDERSALT_API_URL` | `https://api.commandersalt.com` | CommanderSalt API 地址 |
| `EDH_PAGE_URL` | `https://edhpowerlevel.com/` | EDH Power Level 页面地址 |
| `BROWSER_PATH` | 自动查找 | Chrome/Chromium/Edge 可执行文件 |
| `BROWSER_HEADLESS` | `true` | 是否使用无头浏览器 |

## API

```http
POST /api/v1/analyze
Content-Type: application/json

{"url":"https://moxfield.com/decks/<deck-id>"}
```

健康检查：`GET /healthz`。

## 测试

```bash
go test ./...
go build ./cmd/server
```

单元测试使用本地模拟响应，不依赖第三方站点。实际端到端分析会受第三方网站可用性、页面改版及网络环境影响。项目不会绕过验证码或访问控制。
