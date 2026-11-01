# Mihomo Visz

持续采集 Mihomo 连接快照，查看每小时各节点、各域名的上传与下载流量。每秒采集、每 30 秒批量落盘，历史滚动保留 7 天。中文界面，无应用鉴权。默认仅监听 `127.0.0.1:36750`。

前端为 Vite、React 19、shadcn/ui、TanStack Query、Zustand；后端为 Go、`modernc.org/sqlite`，无需 C 编译器。SQLite 数据位于配置的 `data_dir`。

## 启动

需要 Go 1.26.4+、Node.js 24+，以及可选的 [Task](https://taskfile.dev/)。

```bash
cp config.example.yaml config.yaml
# 编辑 config.yaml，设置 Mihomo 地址与 secret_file 路径。
task deps
task build
task run
```

打开 <http://127.0.0.1:36750>。浏览器关闭后，Go 进程仍持续采集。需要保持后端运行才能积累历史；没有自动导入、补全启动前的历史记录。

没有 Task 时：

```bash
go mod download
npm --prefix frontend ci --cache .tmp/npm-cache
npm --prefix frontend run build
mkdir -p .tmp/bin
CGO_ENABLED=0 go build -o .tmp/bin/mihomo-visz ./cmd/mihomo-visz
.tmp/bin/mihomo-visz -config config.yaml
```

`-config` 默认读取当前目录的 `config.yaml`，相对文件路径以该配置文件所在目录为基准。配置采用 `homepage/config.example.yaml` 的 `listen`、`data_dir` 风格，新增 `timezone`、`flush_interval`、`retention_days`、`mihomo` 和 `frontend_dir`。应用没有 OIDC 或登录配置。

`flush_interval` 默认 `30s`，`retention_days` 默认 `7`。采集增量先按“小时 × 节点 × 完整域名”在内存中合并，查询和图表只显示已落盘的数据，通常最多延迟一个落盘周期。正常退出（SIGINT / SIGTERM）会补写最后一批；异常断电或强制终止可能丢失尚未落盘的增量，默认最多约 30 秒。写入失败时保留缓冲并重试，页面会显示存储错误。

启动时和每次落盘时，会在同一事务中清理滚动 168 小时窗口之前的业务流量、全局观测量和采集缺口。小时桶按起始时间过期，边界桶整体删除，因此窗口最早一小时内尚未满 7 天的数据也可能一起删除。跨越窗口边界的缺口会被截断。SQLite 释放的页供后续复用，数据库文件不必立即缩小。

### Mihomo 认证

`mihomo.secret_file` 指向只含 secret 字符串的文件，例如 `/home/eric/.config/mihomo/secret`。由后端程序读取，首尾空白会被去掉，不显示或记录明文，不复制到数据库或返回前端。

也可通过进程环境变量 `MIHOMO_SECRET` 配置，优先于文件。未配置文件和环境变量时使用无认证请求。每次重新连接会重新读取文件，便于凭据更新。

`config.yaml`、数据库、密钥及环境文件应保留在 Git 忽略范围内。`config.example.yaml` 只保存路径，不包含凭据。

### 开发

两个终端分别运行：

```bash
task dev:server    # 后端、采集器：127.0.0.1:36750
task dev:frontend  # Vite：127.0.0.1:5173，/api 代理到后端
```

一个数据目录只运行一个采集进程。开发时复用已运行的后端，不要让两个进程同时采集到同一个数据库。

## 数据如何获得

```text
Mihomo GET /version、GET /proxies
                 ↓
WebSocket GET /connections?interval=1000
                 ↓
排除 Inner → 按连接 ID 计算字节增量 → 分配到小时桶
                 ↓
内存合并 → 每 30 秒事务写入明细、全局观测量、缺口及检查点
                 ↓
清理 7 天窗口外的数据
                 ↓
GET /api/traffic → React Query 自动刷新
```

- `/connections` 的 `upload`、`download` 是累计字节，只有相邻快照的差值会入库。首次启动、重连或全局计数回退时建立新基线，不把此前累计量算进当前小时。
- 首次观察到的新连接，只有开始时间位于当前采样区间才计入其初始流量；已存在但此前未被观察到的连接先建基线。
- 域名优先用 `sniffHost`，其次 `host`；无法识别域名时保留目标 IP，主域名视图归为“未识别域名”。使用 Public Suffix List 归并主域名，完整域名/IP 也可查询。
- 节点依据业务连接自身的 `chains` 和 `/proxies` 类型解析，跳过策略组；无法识别时归为“未识别节点”。不使用策略组当前选择反推历史。
- 使用配置时区划分日期和小时。跨小时的采样增量按区间时长分配，整数余数保留在最后一个桶中，保证字节总和一致。
- 流量单位为十进制：1 GB = 10⁹ 字节。数据库和 CSV 均保存原始字节，上传、下载分别记录。
- “当日全局观测”来自全局累计值的差分，不跟随节点或域名筛选；“已采集流量”来自业务明细，并跟随筛选。两者差额不能可靠归属到某个节点或域名。

### 精度边界

Mihomo 的 WebSocket 推送的是活跃连接快照。两次快照之间建立并关闭的连接，以及最后一次采样后到关闭前的流量，可能漏采。每秒采集无法保证完整覆盖，缺失量不强行补到已知域名。域名归因基于连接元数据；同一连接承载多个域名的复用流量无法进一步逐请求拆分。

后端重启、网络中断、快照超时或全局计数重置，会记录采集缺口。断线后指数退避自动重连；缺口内的历史连接无法从原生 API 恢复。原生 API 也不能还原截图中此前只有总量的历史数据。

如果用了 `dialer-proxy`，内部隧道可能以 `Inner` 类型出现在列表中；它们被排除以避免重复计算。当前统计业务连接报告的出站节点，**不保证能识别机场 → VPS 链中的实际机场入口**。动态链路归属需要内核显式导出父子连接关系；协议开销、机场倍率和实际扣费也不等同于业务字节。

相关官方依据：[API](https://wiki.metacubex.one/api/)、[连接快照路由](https://github.com/MetaCubeX/mihomo/blob/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8/hub/route/connections.go)、[Tracker](https://github.com/MetaCubeX/mihomo/blob/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8/tunnel/statistic/tracker.go)、[内部代理拨号](https://github.com/MetaCubeX/mihomo/blob/3f27ef39f8c96bbfbc8b0b9bb10f5c9ef60149d8/component/proxydialer/proxydialer.go)。本机联调内核版本为 `v1.19.24`。

## 只读约束

Mihomo 客户端仅暴露 `/version`、`/proxies`、`/connections` 三种读取操作，传输层再次拒绝 GET 之外的方法，并禁用 HTTP 重定向。后端不向浏览器提供通用控制器代理。没有切换节点、更改配置、关闭连接或重启内核等操作。

## 查询接口

| 接口 | 用途 |
| --- | --- |
| `GET /healthz` | 进程存活状态 |
| `GET /api/status` | 采集状态、内核版本、活跃/排除连接数、最后采集时间 |
| `GET /api/traffic?date=2026-11-01` | 当日小时明细、节点选项、全局观测和采集缺口 |
| `GET /api/export?date=2026-11-01` | 导出 CSV，上传下载均以字节表示 |

`traffic` 和 `export` 支持 `node`（精确节点名）、`domain`（不区分大小写的字面子串）、`group=domain|host`（默认主域名）。日期按配置时区解析。API 和页面均不进行鉴权。

## 验证

```bash
task test
# 首次运行浏览器测试时安装 Chromium；也可用环境变量指定现有浏览器路径。
cd frontend
PLAYWRIGHT_BROWSERS_PATH=../.tmp/playwright-browsers npx playwright install chromium
PLAYWRIGHT_BROWSERS_PATH=../.tmp/playwright-browsers npm run test:e2e
```

Go 测试覆盖增量去重、跨小时守恒、主域名归并、内部连接排除、计数重置、SQLite 持久化和事务回滚、批量合并与失败重试、无新快照时定时落盘、退出补写、7 天过期清理、API 参数校验及上游只读约束。浏览器测试使用拦截的固定数据验证小时/节点/域名联动、合计一致性、空结果与手机布局，不依赖真实代理流量。浏览器测试结果放在 `.tmp`。
