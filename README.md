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

项目目录约定：`/opt/1panel/docker/compose/my_nav/`

```bash
cd /opt/1panel/docker/compose/my_nav
cp deploy/.env.example .env      # 按需改 NAV_BIND_IP / NAV_PORT
docker compose up -d             # 或 1Panel 界面导入本 compose
```

访问：`http://100.70.0.29:8090`（或 `http://armbian-1.tailbae726.ts.net:8090`）

### 部署前必验三项（MCP 无 shell 通道，需在宿主机执行）

```bash
docker compose version                      # 需要 v2 子命令；只有独立 docker-compose 也可
tailscale status --json | grep CertDomains  # 是否已启用 HTTPS 证书
curl -sI https://t2.gstatic.com/faviconV2   # 语义：出网可达（任意非 000 状态码即可）
```

### 可选：拿到 HTTPS（这样 PWA 才能安装）

浏览器只在**安全上下文**里注册 Service Worker。当前 `http://IP:端口` 方式下 PWA 不可用。
想要 `https://` 而不改动现有 Caddy（它已占用 80/443）：

```bash
# 1) .env 里把绑定改成 NAV_BIND_IP=127.0.0.1
docker compose up -d
# 2) 宿主机执行一次（证书由 Tailscale 自动签发）
sudo tailscale serve --bg --https=8443 http://127.0.0.1:8090
```

之后访问 `https://armbian-1.tailbae726.ts.net:8443`，PWA 可正常安装。

## 目录结构

```
cmd/nav/            入口
internal/config/    环境变量配置
internal/db/        SQLite 打开与 DSN（PRAGMA 必须走 DSN）
internal/migrate/   迁移器（embed *.sql + PRAGMA user_version）
internal/server/    chi 路由装配
internal/web/       go:embed 的前端产物与 SPA 回退
web/                Svelte 5 前端源码
deploy/             compose 与 .env 模板
docs/               规格书与技术调研
```

## 已知约束

- SQLite 必须放在**本地文件系统**（eMMC/ext4）。WAL 在 NFS/CIFS 上会损坏。
- 备份时 `nav.db-wal` 与 `nav.db-shm` 要一起带走，或用 `VACUUM INTO` 生成一致快照。
- 应用内**无认证**（依赖 tailnet 边界）。任何能连上 tailnet 的设备都有写权限。

### ⚠️ 两个会浪费你半小时的坑（都已踩过）

1. **`.sql` 文件必须是纯 ASCII。** sqlc v1.31.1 的 SQLite 解析器遇到多字节 UTF-8 会**静默截断 token**，
   报出完全误导的语法错（`SELECT` 被解析成 `ECT`、`:many` 变成 `:ma`）。
   实测：1072 个非 ASCII 字节 → 8 个假错误；清零 → 干净生成。
   **所以 SQL 注释只写英文，中文解释放 Go 代码或文档里。**

2. **`vite build` 会清空 `internal/web/dist/`**（`emptyOutDir`），把 `.gitkeep` 一并删掉，
   而 `//go:embed` 在目录不存在时是**编译失败**——新克隆会直接 build 不过。
   已在 npm `build` 脚本里构建后补回占位文件。

### 环境依赖

- `make`（本项目的命令入口；若你的机器没有，直接看 Makefile 里的等价命令）
- Node 22+ / Go 1.26+ / Docker（仅部署与镜像构建需要；**本仓库开发机未装 Docker 守护进程**）
- 开发期工具（sqlc 等）由 `make tools` 下载到 `.tools/`，不入库
