# my_nav

自托管的个人导航起始页：图标网格 + iOS 式图标夹 + 多页 + 站内模糊搜索 + 双模式壁纸 + PWA。
单二进制（前端内嵌）+ SQLite，Docker 部署，Tailscale 内网访问。

- **规格书**：[`docs/spec.md`](docs/spec.md)（数据模型 / API 契约 / 交互状态机 / 里程碑 / 验收清单）
- **技术调研**：[`docs/research/`](docs/research/)（四条链路的实测结论与版本来源）

## 技术栈

| 层 | 选型 |
|---|---|
| UI | Svelte 5（runes）+ TypeScript + Vite 8 + Tailwind CSS v4 + `@lucide/svelte` |
| 交互 | `svelte-dnd-action`（拖拽/并夹）+ Fuse.js（模糊搜索） |
| API | Go + chi v5 |
| 数据 | SQLite（`modernc.org/sqlite`，纯 Go，无 CGO）+ sqlc |
| 部署 | Docker（distroless）+ Tailscale |

## 本地开发

```bash
make setup      # 安装前后端依赖
make dev        # Vite :5180 + Go API :8080（/api 已代理）
```

浏览器打开 <http://127.0.0.1:5180>。

> 端口说明：Vite 固定 **5180**（本机 5173 常被其他进程占用）。
> 数据默认落在 `./data/`（`nav.db` + `icons/` + `wallpapers/` + `backup/`）。

## 构建

```bash
make build      # 前端产物 → internal/web/dist → go:embed → bin/nav
./bin/nav       # 单文件可跑，无需 node
```

`internal/web/dist/` 为空时 `go build` 仍可成功（有 placeholder 兜底页），
但请务必先 `make web-build` 才是真正的前端。

> `vite build` 的 `emptyOutDir` 会清空产物目录，因此 build 脚本会在构建后补回
> `internal/web/dist/.gitkeep`——少了它，新克隆的 `go build` 会因为 `//go:embed`
> 找不到目录而直接编译失败。

## 部署（armbian 主机 / 1Panel）

**完整清单见 [`docs/deploy.md`](docs/deploy.md)**（含部署前必验三项、验收清单、升级回滚、
备份恢复、故障排查）。最短路径：

```bash
curl -fsSL https://raw.githubusercontent.com/ffdkj/my_nav/v0.1.0/deploy/install.sh -o /tmp/my_nav-install.sh
sudo bash /tmp/my_nav-install.sh
```

脚本幂等：建目录（`/opt/1panel/docker/compose/my_nav`，与这台机器上其他 compose 项目一致）、
下载 compose、写 `.env`（已存在则不动）、拉镜像、启动、等健康检查、打印访问地址。

访问：`http://100.70.0.29:8090`（或 `http://armbian-1.tailbae726.ts.net:8090`）

### 两种编排变体

| 文件 | 用在什么环境 |
|---|---|
| `deploy/compose.yaml` | 默认：bridge 网络 + 端口映射到 `100.70.0.29:8090`，有网络隔离 |
| `deploy/compose.host-network.yaml` | **容器无法出网时**（宿主用透明代理 TUN/TPROXY，bridge 流量不被接管）：共用宿主网络栈，图标抓取才能工作 |

判断依据：**加任何网址图标都是纯色字母（连国内站也是）** → 用 host 网络变体。
详见 [`docs/deploy.md`](docs/deploy.md) 第 6.5 节。

### 镜像标签

| 标签 | 含义 |
|---|---|
| `0.1.0` / `0.1` | 固定版本（compose 默认用这个） |
| `latest` | 最新发布版 |
| `edge` | main 分支最新构建 |

CI（GitHub Actions）产出 `linux/amd64` + `linux/arm64` 双架构镜像到 `ghcr.io/ffdkj/my_nav`。

### PWA 与 HTTPS 的关系（重要）

Service Worker **只在安全上下文注册**。默认部署是 `http://<tailnet-ip>:8090`，
那里 `serviceWorker.register()` 必然失败——所以应用对失败只记一条 `console.warn`，
**不报错、不阻塞**（PWA 是增强，不是依赖）。想在手机上"添加到主屏幕"，需要 HTTPS：

```bash
# .env 里把绑定改成 NAV_BIND_IP=127.0.0.1，然后
docker compose up -d
sudo tailscale serve --bg --https=8443 http://127.0.0.1:8090
```

之后访问 `https://armbian-1.tailbae726. ts.net:8443` 即可安装 PWA。

缓存策略（vite-plugin-pwa / Workbox）：

| 路径 | 策略 | 原因 |
|---|---|---|
| 应用外壳 | precache | 离线可打开 |
| `/api` GET | **NetworkFirst**（3s 超时） | 导航数据要最新；`CacheFirst` 会给你端上昨天的布局 |
| `/api` 写请求 | NetworkOnly | 绝不重放写操作 |
| `/icons`、`/wallpapers` | CacheFirst | 内容寻址，路径变则内容必变 |

**一个容易忽略的坑**：Workbox 的运行时缓存只在 **GET** 时更新，而改布局走的是 PUT。
不管的话，用户刚加的图标一断网就"消失"（离线命中的是上次 GET 的旧布局）。
所以前端在每次提交成功后会把权威结果**写回同一份缓存**（应用侧写穿）。
这条有 e2e 覆盖（离线重载后图标仍在）。

### 备份与迁移

设置 →「数据」分区：

- **导出 JSON**：人可读、可手改（想删一个图标就直接删 `items` 里那一行），适合版本管理
- **导出 zip**：含图标与壁纸二进制，换机器时用它才能完整还原
- **导入**：**全量覆盖**（替换所有页面、图标与设置）。导入前服务器会自动写一份快照
  `/data/backup/pre-import.json`——**永久只保留最新一份**，可在同一分区下载
- 快照只在**校验通过**后才写：坏数据会在动手之前被拒，现状与快照都不受影响

导入顺序刻意如此：先校验 → 再写快照 → 再落资源文件（内容寻址，纯增量）→ 最后单事务换库。

### 环境依赖

- `make`（本项目的命令入口；若你的机器没有，直接看 Makefile 里的等价命令）
- Node 22+ / Go 1.26+ / Docker（仅部署与镜像构建需要；**本仓库开发机未装 Docker 守护进程**）
- 开发期工具（sqlc 等）由 `make tools` 下载到 `.tools/`，不入库
