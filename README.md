# Commander Power Check

输入公开的 Moxfield Commander 牌组 URL 或粘贴牌表文本，同时查询 CommanderSalt 与 EDH Power Level 的评分。

## 功能

- **双评分独立展示**：CommanderSalt 与 EDH Power Level 分别给出 Bracket（规则值 / 评估值）与 Power Level，不计算平均值。
- **EDH Power Level 判定细节**：展示 Game Changers、早期 2-Card Combo、Extra Turns、Mass Land Denial 的具体数量与判定依据。
- **构筑指标与缺口报告**：按地牌、规划、群体互动、单体互动、抽牌/弃牌、加速六类统计，显示当前数量与缺口。
- **EDHREC 缺口推荐**：只推荐能补足当前短缺类别的单卡，并说明每张卡的填补理由。
- **单卡替换预览**：选择一张移除牌与一张加入牌，对比替换前后的构筑指标、牌张数与基础合法性，不修改 Moxfield、不保存版本。
- **卡图与关联数据**：Scryfall 卡图（含双面牌正反面切换）、Commander Spellbook 组合、EDHREC 推荐。
- **轻量组牌编辑器**：在本平台内增删卡牌并导出牌表文本。

## 工作方式

1. 后端严格校验 `https://moxfield.com/decks/{deck-id}` 地址；也可粘贴牌表文本作为标准牌表来源。
2. CommanderSalt 直接接收该 URL，返回评分和标准化牌表。
3. 后端将牌表转换成与 Moxfield `Export → Copy Plain Text` 等价的文本。
4. 后端复用一个 Chromium 进程，通过有界的独立标签页分析 EDH Power Level，并提取结果。
5. 相同 Deck ID 的并发请求会合并；结果使用有容量上限的 TTL/LRU 缓存。
6. 两个评分模型分别展示，不计算平均值。EDH Power Level 失败时仍返回 CommanderSalt 结果。

## 本地运行

Windows 用户把仓库 clone 下来后，直接双击根目录的 `start.cmd` 即可启动并自动打开页面，完成后双击 `stop.cmd` 关闭。仓库内已附带编译好的 `server.exe`，**无需安装 Go**、Chrome 或 Git；脚本也会自动查找本机的 Chrome / Chromium / Microsoft Edge。

如果你本机装有 Go 1.24+，删除 `server.exe` 后首次双击也会从源码现场编译（方便你自己改动代码后重跑）：

```bash
go run ./cmd/server
```

打开 <http://localhost:8080>。

开发时推荐使用重启脚本。它会先构建新版本；构建成功后，只终止由该脚本记录并验证过的旧服务进程，再启动新服务并等待健康检查通过：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\restart.ps1
```

脚本使用 `.run/powerlevel.pid` 管理进程，不会根据端口或进程名称误杀其他程序。运行日志位于 `.run/powerlevel.log` 和 `.run/powerlevel.error.log`。直接使用 `go run` 启动的旧进程不受该脚本管理，需要先手动停止一次；此后统一使用重启脚本即可自动清理。

Windows 会自动尝试寻找 Edge；其他平台建议设置浏览器路径：

```bash
BROWSER_PATH=/usr/bin/chromium go run ./cmd/server
```

## 配置

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `APP_ADDRESS` | `:8080` | 监听地址 |
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

{"url":"https://moxfield.com/decks/d8xoDMBAcUOKmd8VjkU66w"}
```

另有健康检查：`GET /healthz`。

## 生成 Windows 免安装包

可生成解压即用的 Windows x64 发布包：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-release.ps1 -Version 0.1.0
```

输出位于 `dist/PowerLevel-windows-amd64-0.1.0.zip`，同时生成 SHA-256 文件。解压后双击 `启动工具.cmd`，使用完成后双击 `关闭工具.cmd`，无需安装 Go 或 Git。

> 提示：项目根目录的 `server.exe` 是为了让 clone 后零环境依赖即可双击运行而提交的预编译二进制。每次改动源码后记得重新构建并提交它（`go build -trimpath -ldflags "-s -w" -o server.exe ./cmd/server`），否则仓库里的二进制会和源码异步。

## 测试与构建

```bash
go test ./...
go build ./cmd/server
```

单元测试使用本地模拟响应，不依赖第三方站点。实际端到端分析会受第三方网站可用性、页面改版及网络环境影响。项目不会绕过验证码或访问控制。
