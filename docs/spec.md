# my_nav 规格书（v1，待确认）

> 个人导航起始页（Docker 部署 / Tailscale 内网）。本文件是实现前的唯一事实源；
> 凡标注 **【待确认】** 的条目必须先闭环，其余均为已锁定决策。
> 技术结论均有实测来源，见 `docs/research/01–04-*.md`。

---

## 0. 目标与非目标

**目标**：一个自托管的个人导航页。图标网格 + 多页 + iOS 式图标夹（含 2×2 可交互大夹）+ 自由拖拽吸附 + 站内模糊搜索与搜索引擎切换 + 双模式壁纸 + PWA 可安装 + JSON/zip 导入导出。

**非目标（v1 明确不做）**：多用户与账号系统、分享/协作、点击统计与最近访问、RSS/天气等小组件、文件夹嵌套、跨设备实时同步（多人同时编辑）、离线写操作（离线只读缓存）。

**使用场景**：单人、Tailscale tailnet 内、手机与桌面浏览器、`https://nav.tailbae726.ts.net`。

---

## 1. 已锁定决策

| # | 决策 | 取值 |
|---|---|---|
| 1 | 访问控制 | **零应用认证**（tailnet 内全权限）；不实现 `NAV_READONLY` |
| 2 | 部署形态 | Go 单容器（`go:embed` 内嵌 SPA），把 8080 发布到 **Tailscale 虚拟 IP 的高位端口**（默认 `100.70.0.29:8090`）；其他设备直接 `http://IP:端口` 访问。**不起 Tailscale 边车、不新增 tailnet 节点、不改现有 Caddy** |
| 3 | 布局模型 | **顺序即布局**：数据只保存一条有序序列，`(col,row)` 由打包器算出（提交时按 12 列，渲染时按当前显示列数）——冲突/空洞/折行天然不存在；2×2 夹由打包器预留 4 格 |
| 4 | 多页 | 底部 iOS 圆点；可命名/增删/排序；URL `#/p/<slug>`；壁纸每页独立或跟随全局（设置可切） |
| 5 | 翻页 | 点圆点 / 滚轮 / 手势横滑（**左滑=下一页，右滑=上一页**）/ **拖拽悬停屏幕左右边缘**（阈值 `page_flip_edge_ms`，默认 150ms，仅拖拽中生效） |
| 6 | 文件夹 | 单层不嵌套；容量 **9**；空夹自动删；小夹 1 格；大夹 2×2 格 |
| 7 | 小夹外观 | **固定 9 个缩略图**（不降级）；点击整夹 → 模态（壁纸虚化、可内部拖拽排序、拖出即回主网格） |
| 8 | 大夹外观 | 内部 **9 个图标直接可点**；编辑态才可拖拽管理；编辑面板开关切换 1 格 ⇄ 2×2 |
| 9 | 占位冲突 | 被挤占项**顺延到最近空位**；文件夹不因内容少而自动缩回 |
| 10 | 合并触发 | 悬停在目标图块上达 **500ms**（设置可调）后**武装**合并（目标放大高亮），**松手时执行**；中途移开即取消。不在拖拽中途改结构，避免拖拽库状态错乱 |
| 11 | 跨页搬图标 | **边缘悬停翻页 + 拖拽继续**：合成一次 `mouseup` 让拖拽库体面收场 → 切页 → 自研漂浮层接管指针 → 松手走**原子端点** `POST /api/board/move`（它接收**目标页搬完后的完整布局**，在单事务里既搬又重排目标页其余项）。高风险项 R2，已用真浏览器验收 |
| 12 | 图标抓取 | 添加时**后端同步抓取**；链路 `faviconV2` → DuckDuckGo `ip3` → 自建 HTML 发现 → 纯色文字兜底 |
| 13 | 图标存储 | **原样字节**存盘（内容寻址），不转码；DB 记 MIME/尺寸/来源 |
| 14 | 图标兜底 | 编辑面板：重新抓取 / 上传本地（≤512KB，缩到 256×256）/ 纯色文字（10 预设色 + 取色器）/ 打开原站 / 重置为标准 favicon |
| 15 | 搜索框 | 输入即下拉**全局**站内匹配；**回车 = 当前引擎搜索**；`↑↓` 高亮 + **`Ctrl/Cmd+Enter` 打开高亮项**；徽标点击弹引擎面板 |
| 16 | 引擎 | 内置 6 个 + 完全自定义（名称/图标文字/颜色/URL 模板含 `{query}`）；默认引擎存服务端 |
| 17 | 壁纸 | 上传到服务器（≤10MB，生成 1920 宽 WebP）+ 图床 URL 直链（附"下载到服务器"）；多张 + 随机轮换可开关；兜底纯色/渐变可配 |
| 18 | 导入导出 | `GET /api/export`（可读 JSON）、`?withAssets=1`（zip）；导入 = **全量覆盖**；导入前写**唯一一份** `/data/backup/pre-import.json`；设置页显示其时间 + 下载按钮 |
| 19 | 数据库 | SQLite（`modernc.org/sqlite`，CGO-free）；**sqlc v1.31.1** 生成代码入库；手写 `embed` 迁移 + `PRAGMA user_version` |
| 20 | 数据卷 | 单 `/data` 卷：`nav.db`(+`-wal`/`-shm`) / `icons/` / `wallpapers/` / `backup/` |
| 21 | 前端 | Svelte 5.57 runes / Vite 8.3 / Tailwind v4（CSS-first）/ `@lucide/svelte` / **svelte-dnd-action 0.9.79** / Fuse.js 7.5 / vite-plugin-pwa 1.3 |
| 22 | 镜像交付 | **GitHub Actions 构建多架构镜像 → ghcr.io**；宿主只 `pull` |
| 23 | 落盘位置 | `/opt/1panel/docker/compose/my_nav`（1Panel 可识别管理） |
| 24 | 管理方式 | 1Panel「容器/编排」界面；compose 文件为唯一事实源，可被面板导入 |

| 25 | 拖拽门控 | **无编辑态**：拖拽随时可用；**触屏必须按住 300ms 才起拖**（鼠标即时），以此消除误触 |
| 26 | 新增/删除入口 | 网格末尾常驻虚线 `+` 图块；删除 = 长按 800ms 上下文菜单 / 桌面悬停角标 `×` + 确认 |
| 27 | PWA | 当前 http 访问下**不可安装**（非安全上下文）；组件已按可选实现，升级 HTTPS 后自动生效 |

---

## 2. 技术栈版本锁定

| 层 | 组件 | 版本 | 备注 |
|---|---|---|---|
| 前端 | svelte | 5.57.0 | runes；`$state` 共享状态必须写在 `.svelte.ts` |
| | vite | 8.3.0 | |
| | @sveltejs/vite-plugin-svelte | 7.3.0 | 要求 vite ^8 + svelte ^5.46.4 |
| | tailwindcss + @tailwindcss/vite | 4.3.3 | **无 `tailwind.config.js`**，`@theme` 写在 CSS 里 |
| | @lucide/svelte | 1.47.0 | ⚠️ **不是** `lucide-svelte`（那是 Svelte 4 线） |
| | svelte-dnd-action | 0.9.79 | ≥0.9.77；items 必须是 `$state` 数组里的**普通对象** |
| | fuse.js | 7.5.0 | `ignoreLocation: true`、`threshold 0.35` |
| | vite-plugin-pwa | 1.3.0 | workbox 7.4.1；`registerType: 'prompt'` |
| | typescript | 6.0.x（模板锁定） | 7.0.2 与 svelte-check 兼容性未验证，**跟随模板** |
| 后端 | go | 1.26（宿主 1.26.7 / 最新 1.27.1） | `CGO_ENABLED=0` 静态 |
| | github.com/go-chi/chi/v5 | v5.3.2 | ⚠️ **禁用 `middleware.RealIP`**（已被 3 条安全公告弃用） |
| | modernc.org/sqlite | v1.59.0 | 驱动名 `"sqlite"`；PRAGMA 必须写进 DSN（逐连接生效） |
| | sqlc | v1.31.1 | `engine: "sqlite"`（Beta）、config `version: "2"`、`emit_pointers_for_null_types: true` |
| | golang.org/x/image | v0.46.0 | 仅 webp 解码；**不引入 ICO 解码库**（原样存字节，浏览器自会渲染） |
| 基础设施 | tailscale/tailscale | 指定 tag | 边车节点，`TS_AUTH_ONCE=true` + `TS_STATE_DIR` |
| | ghcr.io | — | CI 产物 |
| 开发机 | Node 22.23.2 / pnpm 11.22 | 本机实测 | ⚠️ 5173 被占 → Vite 用 **5180** |
| | Docker | **本机守护进程不可达** | 容器验证只在 armbian 主机做 |

---

## 3. 部署拓扑

```
   其他设备（手机/笔记本，同在 tailnet）
                 │
                 │  http://100.70.0.29:8090   （或 http://armbian-1.tailbae726.ts.net:8090）
                 ▼
        ┌────────────────────────────────────────────┐
        │  宿主 armbian-1 (aarch64, eMMC ext4)        │
        │  80/443 由既有 Caddy 独占（vaultwarden 在用）│
        │                                            │
        │  ┌──────────────────────────────────────┐  │
        │  │ 容器 app (Go, distroless, 非 root)    │  │
        │  │   listen :8080                       │  │
        │  │   /data ← ./data (bind mount)        │  │
        │  └──────────────────────────────────────┘  │
        │        ports: 100.70.0.29:8090 → 8080      │
        └────────────────────────────────────────────┘
```

- **不新增 tailnet 节点、不改动既有 Caddy/1Panel/vaultwarden**；只占用一个高位端口。
- **代价（已确认接受）**：`http://` 不是安全上下文 → **Service Worker 不注册、PWA 不可安装**。
- **可选升级（1 条命令，不改 compose）**：`.env` 把 `NAV_BIND_IP` 改为 `127.0.0.1`，
  宿主机执行 `sudo tailscale serve --bg --https=8443 http://127.0.0.1:8090`
  → 得到 `https://armbian-1.tailbae726.ts.net:8443`，PWA 立即可用（证书由 Tailscale 自动签发）。
- 落盘：`/opt/1panel/docker/compose/my_nav/`（compose + `.env`），数据 `/opt/1panel/docker/compose/my_nav/data/` → 容器 `/data`。
- 存储：eMMC ext4 本地盘（WAL 安全）；**禁止**把 `/data` 放到 NFS/CIFS。
- 日志：宿主 `/var/log` 在 zram（重启即失），审计不依赖容器日志。

---

## 4. 目录结构

```
my_nav/
├─ cmd/nav/main.go                 # 入口：配置、DB、迁移、路由、优雅退出
├─ internal/
│  ├─ config/config.go             # 环境变量 → Config
│  ├─ server/                      # chi 路由装配、中间件、SPA 回退、静态资源缓存头
│  │  ├─ router.go
│  │  ├─ handlers_page.go          # pages / board
│  │  ├─ handlers_link.go
│  │  ├─ handlers_icon.go          # 抓取 / 上传 / 单色字图标
│  │  ├─ handlers_engine.go
│  │  ├─ handlers_wallpaper.go
│  │  ├─ handlers_settings.go
│  │  ├─ handlers_export.go        # export / import / backup
│  │  └─ middleware.go
│  ├─ db/                          # sqlc 生成物（入库，CI 不需 sqlc 二进制）
│  │  ├─ db.go  models.go  queries.sql.go
│  │  └─ sql/{schema.sql, query.sql}
│  ├─ migrate/                     # 手写迁移器
│  │  ├─ migrate.go
│  │  └─ migrations/001_init.sql   # 零填充，词法序 = 数字序
│  ├─ favicon/                     # 抓取链、SSRF 防护、校验、内容寻址落盘
│  │  ├─ fetch.go  discover.go  store.go  ssrf.go
│  ├─ imageproc/                   # 壁纸缩放/WebP、尺寸与像素上限校验
│  ├─ export/                      # JSON / zip 导出与导入
│  └─ web/                         # go:embed all:web  (构建期注入 SPA 产物)
├─ web/                            # 前端（Svelte 5 + Vite）
│  ├─ src/
│  │  ├─ main.ts  app.css
│  │  ├─ App.svelte
│  │  ├─ lib/
│  │  │  ├─ api.ts                 # 类型化 fetch 封装（含错误归一）
│  │  │  ├─ types.ts               # 与后端契约共享（手写，单一来源）
│  │  │  ├─ store/                 # *.svelte.ts 单例（runes）
│  │  │  │  ├─ board.svelte.ts     # 当前页 board + 乐观更新 + 撤销栈
│  │  │  │  ├─ edit.svelte.ts      # 编辑态、选中项、模态栈
│  │  │  │  ├─ dnd.svelte.ts       # 拖拽会话、merge dwell、跨页 carry
│  │  │  │  └─ settings.svelte.ts  # 设置/引擎/壁纸/主题
│  │  │  └─ components/
│  │  │     ├─ Grid.svelte  Tile.svelte  FolderTile.svelte  BigFolder.svelte
│  │  │     ├─ FolderModal.svelte  SearchBar.svelte  EnginePicker.svelte
│  │  │     ├─ PageDots.svelte  PageManager.svelte  LinkEditor.svelte
│  │  │     ├─ IconMonogram.svelte  WallpaperLayer.svelte
│  │  │     └─ Toast.svelte  Confirm.svelte  UpdatePrompt.svelte
│  │  └─ pwa.ts                    # virtual:pwa-register
│  ├─ index.html  vite.config.ts  svelte.config.js  tsconfig*.json
│  └─ public/{pwa-192x192.png, pwa-512x512.png, favicon.svg, apple-touch-icon.png}
├─ deploy/
│  ├─ compose.yaml                 # app + tailscale 边车（生产）
│  ├─ compose.dev.yaml             # 仅 app，端口 127.0.0.1:8080
│  ├─ tailscale/serve.json         # TS_SERVE_CONFIG（目录挂载！）
│  └─ .env.example
├─ docs/{spec.md, research/01..04-*.md, deploy-checklist.md}
├─ .github/workflows/release.yml   # 多架构构建 → ghcr.io
├─ Dockerfile                      # 多阶段：node 构建 SPA → go 构建 → distroless
├─ sqlc.yaml  go.mod  go.sum  Makefile  README.md
```

---

## 5. 数据模型

### 5.1 主键策略
所有实体主键为 **客户端生成的 UUIDv7 文本**（26/36 字符）。好处：布局可整板声明式提交（幂等）、离线乐观 UI、导入导出可直接对齐、无需服务端返回 ID 再回填。

### 5.2 表结构（`001_init.sql`）

```sql
PRAGMA foreign_keys = ON;

CREATE TABLE pages (
  id             TEXT PRIMARY KEY,
  slug           TEXT NOT NULL UNIQUE,
  name           TEXT NOT NULL,
  sort_order     INTEGER NOT NULL,
  wallpaper_mode TEXT NOT NULL DEFAULT 'global' CHECK (wallpaper_mode IN ('global','custom')),
  wallpaper_id   TEXT REFERENCES wallpapers(id) ON DELETE SET NULL,
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE links (
  id            TEXT PRIMARY KEY,
  title         TEXT NOT NULL,
  url           TEXT NOT NULL,
  open_new_tab  INTEGER NOT NULL DEFAULT 1,
  icon_source   TEXT NOT NULL DEFAULT 'auto' CHECK (icon_source IN ('auto','upload','monogram')),
  icon_path     TEXT,            -- 相对 /data/icons，内容寻址
  icon_mime     TEXT,
  icon_w        INTEGER,
  icon_h         INTEGER,
  icon_status   TEXT NOT NULL DEFAULT 'pending' CHECK (icon_status IN ('pending','ok','miss','error')),
  icon_checked_at TEXT,
  mono_text     TEXT, mono_color TEXT NOT NULL DEFAULT '#3B82F6', mono_font_size INTEGER NOT NULL DEFAULT 30,
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE folders (
  id         TEXT PRIMARY KEY,
  name       TEXT,
  size       INTEGER NOT NULL DEFAULT 1 CHECK (size IN (1,2)),  -- 1 = 1×1, 2 = 2×2
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- placement：唯一的"位置"事实源
CREATE TABLE placements (
  id          TEXT PRIMARY KEY,
  page_id     TEXT NOT NULL REFERENCES pages(id)   ON DELETE CASCADE,
  folder_id   TEXT          REFERENCES folders(id) ON DELETE CASCADE, -- 非空 = 该 placement 是一个文件夹
  link_id     TEXT          REFERENCES links(id)   ON DELETE CASCADE, -- 非空 = 该 placement 是一个链接
  in_folder   TEXT          REFERENCES folders(id) ON DELETE CASCADE, -- 非空 = 该链接位于此文件夹内
  col         INTEGER NOT NULL DEFAULT 0,
  row         INTEGER NOT NULL DEFAULT 0,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  CHECK ((link_id IS NULL) <> (folder_id IS NULL)),
  CHECK (folder_id IS NULL OR in_folder IS NULL)   -- 禁止嵌套
);

CREATE INDEX idx_place_page   ON placements(page_id, in_folder);
CREATE INDEX idx_place_folder ON placements(in_folder, sort_order);

CREATE TABLE engines (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  url_tpl    TEXT NOT NULL,       -- 必须包含 {query}
  icon_text  TEXT NOT NULL, icon_color TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  is_builtin INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE wallpapers (
  id          TEXT PRIMARY KEY,
  kind        TEXT NOT NULL CHECK (kind IN ('upload','url')),
  remote_url  TEXT,               -- kind=url
  file        TEXT,               -- kind=upload（相对 /data/wallpapers）
  thumb_file  TEXT,               -- 1920 宽 WebP
  w INTEGER, h INTEGER,
  bytes       INTEGER,
  sort_order  INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE settings (k TEXT PRIMARY KEY, v TEXT NOT NULL);
```

**设置项（settings 表 key）**：`default_engine_id`、`merge_dwell_ms`（500）、`page_flip_edge_ms`（150）、`wallpaper_mode`（`global|custom`）、`wallpaper_id`、`wallpaper_rotation`（`off|load|interval`）、`wallpaper_interval_min`（30）、`wallpaper_fallback`（`#0b1220`）、`search_open_new_tab`、`theme`（`dark|light|auto`）、`reduced_motion`、`schema_note`。

### 5.3 不变量（服务端强制）
1. 文件夹**不嵌套**；`in_folder` 指向的文件夹必须属于同一 `page_id`。
2. 同一文件夹内 `link_id` 数量 ≤ **9**。
3. 同一页面上 placement 占用格**不得重叠**（2×2 夹占 4 格）；冲突由客户端顺延，服务端**拒绝**并返回冲突格。
4. `folders.size=2` 的 placement 必须有 `col+1 < GRID_COLS`。
5. 页面至少保留 1 个；删除页面级联删除其 placements（links 实体保留，除非显式删）。
6. 空文件夹（无 in_folder 子项）在**同一次写操作内**自动删除。

### 5.4 迁移
`internal/migrate`：`//go:embed migrations/*.sql`，按文件名排序，`PRAGMA user_version` 记录版本，逐文件单事务执行（`PRAGMA user_version = N` 用**校验过的整数插值**，不能绑定参数）。启动时自动执行；执行前若版本将变化，先 `VACUUM INTO /data/backup/pre-migrate.db`（**覆盖式，只留一份**）。

---

## 6. API 契约

统一前缀 `/api`；JSON；错误体 `{"error":{"code":"...","message":"..."}}`；
未知 `/api/*` 路径返回 **JSON 404**（在 `/api` 分组内注册 `NotFound`，避免落到 SPA 回退）。

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/healthz` | 存活探针（Docker healthcheck） |
| GET | `/api/bootstrap` | 一次性拉取：pages、settings、engines、wallpapers、版本号 |
| GET | `/api/pages/{id}/board` | 该页完整 board（placements + folders + links） |
| PUT | `/api/pages/{id}/board` | **整板声明式替换**（布局/建夹/合并/移出/排序全走这里，单事务） |
| POST | `/api/board/move` | **跨页原子搬移**：`{from_page_id, to_page_id, item_id, items:[目标页完整布局]}`；同时 bump 两页 revision |
| POST | `/api/pages` | 建页 `{id,slug,name,after_id?}` |
| PATCH | `/api/pages/{id}` | 改名/slug/壁纸模式/排序 |
| DELETE | `/api/pages/{id}` | 删页（最后一页拒绝） |
| POST | `/api/links` | 建链接 `{id,url,title?,page_id,col,row}` → 同步抓图标后返回（可能 `icon_status=miss`） |
| PATCH | `/api/links/{id}` | 改标题/URL/打开方式/单色字字段（改 URL 触发重新抓取，除非 `icon_source!=auto`） |
| DELETE | `/api/links/{id}` | 删链接（同时清 placement） |
| POST | `/api/links/{id}/icon/refetch` | 手动重新抓取（忽略负缓存） |
| POST | `/api/links/{id}/icon/upload` | multipart 上传本地图标（≤512KB，缩 256×256） |
| POST | `/api/links/{id}/icon/monogram` | 切纯色文字（`{text,color,font_size}`） |
| POST | `/api/links/{id}/icon/reset` | 重置为标准 favicon（`icon_source=auto` + 重新抓取） |
| GET/POST/PATCH/DELETE | `/api/engines[/{id}]` | 引擎 CRUD（内置项可改排序，不可删） |
| GET/POST/PATCH/DELETE | `/api/wallpapers[/{id}]` | 上传/URL 添加、排序、删除 |
| POST | `/api/wallpapers/{id}/materialize` | 把图床 URL 下载固化到本地 |
| GET/PATCH | `/api/settings` | 设置读写 |
| GET | `/api/export` | 可读 JSON（图标/壁纸以相对路径引用） |
| GET | `/api/export?withAssets=1` | zip：`data.json` + `icons/` + `wallpapers/{orig,thumb}/` |
| POST | `/api/import` | multipart（json 或 zip）→ **全量覆盖**；**校验通过后**才写 `pre-import.json` |
| GET | `/api/backup` | `{exists, at, bytes}`（设置页展示） |
| GET | `/api/backup/download` | 下载 `pre-import.json` |
| GET | `/icons/{path}` | 图标（内容寻址 + 强缓存 + `nosniff`） |
| GET | `/wallpapers/{orig\|thumb}/*` | 壁纸原图/缩略图（通配路由；内容寻址的路径是两段） |

### 6.1 PUT board 请求体（核心）

```json
{
  "revision": 42,
  "items": [
    { "id": "pl_01H..a", "kind": "link", "link_id": "ln_01H..1", "col": 0, "row": 0 },
    { "id": "pl_01H..b", "kind": "folder", "folder_id": "fd_01H..1", "size": 2, "col": 3, "row": 0,
      "children": [ { "id": "pl_01H..c", "link_id": "ln_01H..2", "sort_order": 0 } ] }
  ],
  "new_links": [ { "id": "ln_01H..9", "url": "https://x.dev", "title": "X" } ],
  "new_folders": [ { "id": "fd_01H..1", "name": "Dev", "size": 2 } ],
  "deleted_link_ids": [], "deleted_folder_ids": []
}
```

- 服务端在**单事务**内：建实体 → 删实体 → 校验不变量 → 全量替换该页 placements → 递增 `revision`。
- `revision` 不匹配返回 `409`（同一时间只手改；多人场景不在 v1 范围）。
- 校验失败返回 `422` + `conflicts[]`（`{col,row}`），前端据此重排后重试一次。

---

## 7. 图标抓取子系统

### 7.1 抓取链（后端同步执行，总预算 3s）

| 步 | 目标 | 成功 | 失败判定 | 超时 |
|---|---|---|---|---|
| 1 | `https://t2.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url=<完整带 scheme 的 URL>&size=128` | 200 + `image/png` | **404 = 无图标**（726B 地球 PNG 哨兵）；`sz` 只是提示，可能只给 32/16px | 1.2s |
| 2 | `https://icons.duckduckgo.com/ip3/<host>.ico` | 200 + `image/x-icon` | 404 = 无 | 1.0s |
| 3 | 自建发现：GET 站点 HTML → 解析 `<link rel="icon"\|shortcut icon\|apple-touch-icon\|mask-icon>`（按研究里的优先级，href 依 `<base href>` 解析）→ 取最大/最合适；再兜 `/favicon.ico` | 200 + 可解析图片 | 无 link 且 `/favicon.ico` 非 200 | 1.5s |
| 4 | 失败 → `icon_status=miss`，前端直接打开**纯色文字**兜底面板 | — | — | — |

- 第 1 步返回 <64px 时才跑第 3 步尝试升级（避免无谓请求）。
- `s2/favicons` **不直接用**：它只是 301 跳板、body 是 HTML，等价于 `faviconV2` 多一跳。
- **负结果缓存**：`icon_status=miss` + `icon_checked_at`，30 天内不自动重试（手动"重新抓取"可绕过）。
- 自定义 `User-Agent`（如 `my_nav/1.0 (+personal)`）；全局并发信号量 **4**。

### 7.2 存储与校验
- 内容寻址：`/data/icons/<sha256[0:2]>/<sha256>.<ext>`（ext 由嗅探 MIME 决定），DB 存相对路径 → 同图标自动去重。
- 大小上限 **2 MiB**（`io.LimitReader`，超限即拒）；`image.DecodeConfig` 取尺寸并做**像素上限**（≤ 4096×4096，防解压炸弹）；`http.DetectContentType` 仅作预筛（**不认识 ICO**）。
- SVG：接受但仅作 `<img>` 使用（不执行脚本）；响应头 `Content-Type: image/svg+xml` + `X-Content-Type-Options: nosniff` + `Content-Security-Policy: default-src 'none'; style-src 'unsafe-inline'`；直接访问不构成 XSS 面。
- ICO：**不引入解码库**，原样存 `image/x-icon` 由浏览器渲染；仅在需要尺寸时读 ICONDIR 头。

### 7.3 SSRF 防护（第 3 步必须）
- `net.Dialer.Control` 校验**实际拨号 IP**：拒绝 loopback / RFC1918 / link-local / multicast / unspecified（覆盖 DNS rebinding）。
- 只允许 `http`/`https`、端口 80/443；拒绝带 userinfo 的 URL；`CheckRedirect` 跳数 ≤3（每跳重新校验）。
- 请求 context 继承入站请求（客户端断开即取消）。

---

## 8. 壁纸子系统
- 上传：≤10MB，接受 png/jpg/webp/gif（按解码结果嗅探，不用 `http.DetectContentType`）；原图按真实格式存 `/data/wallpapers/orig/`，缩略图为 1920 宽 **JPEG q82** 存 `/data/wallpapers/thumb/`。
  ⚠️ 与原计划的偏差：缩略图用 JPEG 而非 WebP —— `golang.org/x/image` 只有 WebP **解码**、没有编码器，纯 Go 编码 WebP 需要 cgo 或第三方实现，会赔掉 `CGO_ENABLED=0` 静态单二进制的优势。壁纸是照片类内容，JPEG 足够。
- 两者均为**内容寻址**（sha256 路径）⇒ 重复上传不占两份；删除时按引用计数清理文件（eMMC 只有十几 G）。
- URL 模式：前端直链（省服务器流量），提供"下载到服务器"（`materialize`，走同一 SSRF 防护与大小上限）。
- 多张 + 排序；轮换模式 `off | load（每次进入随机）| interval（每 N 分钟）`。
- 每页 `wallpaper_mode = global | custom`；全局壁纸在无自定义的页面上生效。
- 兜底：纯色/渐变可配（`wallpaper_fallback`）；图片 404 时自动降级。
- 渲染：固定定位图层 + `background-attachment: fixed` 语义；`prefers-reduced-motion` 时关闭交叉淡入。

---

## 9. 前端架构要点

- **状态**：`*.svelte.ts` 里的 class 单例（`board`、`edit`、`dnd`、`settings`）；组件通过 `$props()` 接收，不用 store API。
- **乐观更新**：拖拽/合并在内存立即生效，PUT board 失败则回滚 + toast。
- **撤销栈**：内存保留最近 20 步（同一页 board 快照）→ `Ctrl/Cmd+Z`。
- **设计令牌**：Tailwind v4 `@theme` 定义色板/圆角/阴影；暗色用 `@custom-variant dark (&:where(.dark, .dark *))` + `<html class="dark">` 由 `$effect` 驱动。
- **图块尺寸**：CSS 变量 `--tile`（桌面 96px / 平板 84px / 手机 72px），12 列逻辑网格，显示列数 = `min(12, floor(可用宽 / (tile+gap)))`，超出列宽的项按 `row-major` 折行。
- **无障碍**：`<nav aria-label>` + `<li>` 包真实 `<a href>`（不用 `role="grid"`）；每个 dndzone 与每个可拖项都有 `aria-label`；合并动作走 `aria-live="polite"` 播报（库不认识我们的合并状态变化）；`setKeyboardDragTrigger('space')` 以保住 Enter 打开链接。

---

### 9.1 写调度器（为什么前端需要一个"提交队列"）

图标抓取把单次 PUT 从毫秒级拉长到**最长 3 秒**，于是原本窗口极小的竞态变成必现。
e2e 一次性暴露了三个同源问题，最终设计固定为
**脏标记 + 串行 + 发送时构建负载 + 页面绑定**：

| 若这样做 | 会出的问题 |
|---|---|
| 并发提交（不串行） | 后一次带过期 revision → **409**，用户看到"加了又没了" |
| 在"排队时"构建负载 | 负载里的 revision 是旧的 → **409** |
| 在"飞行期间"构建负载 | 把已经发出去的 `new_links` 再声明一遍 → **422 链接已存在** |
| 用响应整块替换本地状态 | 飞行期间的新改动被静默抹掉（**整页图标消失**） |
| 清理空文件夹时用旧快照判断 | 把期间新建的文件夹误删，下一次提交发出**空布局** |

因此：`commit()` 只置脏标记；`#flush()` 串行取最新状态构建负载；
`revision` 从**按页面维护的 revision 表**里在发送那一刻读取；
响应只用于**合并**（吸收 revision 与抓取结果），不覆盖本地布局；
切页前先 `await settled()` 把队列排空。

这条经验也说明：**给任何"乐观更新 + 声明式整板提交"的系统加慢 I/O，先检查写路径是否可重入。**

## 10. 交互状态机

### 10.1 页面级（无编辑态；Q29 已定）
```
触屏:  手指按下 ──300ms 未移动──▶ 进入可拖拽（图块抬起放大反馈）
                       │
                       ├─ 移动 → 拖拽 / 合并 / 拖到边缘翻页
                       └─ 继续按住到 800ms 仍不动 → 弹出上下文菜单（编辑 / 删除 / 移动到… / 放大为 2×2）
鼠标:  按下即拖（`delayTouchStart` 只作用于触屏，符合 svelte-dnd-action 的命名与语义）

常驻交互: 点击链接=跳转; 点击小夹=模态; 点击大夹内图标=跳转; 滚轮/横滑=翻页
        网格末尾虚线 `+` 图块 = 新增链接
        桌面端悬停图块右上角浮出 `×` = 删除（带确认）
        底部圆点区右侧常驻 `⋯` = 页面管理（改名/增删/排序）
```
**取消与安全**：`ESC` / `pointercancel` / 窗口失焦 → 一律取消拖拽并还原，不落盘。

### 10.2 布局打包器（`web/src/lib/layout.ts`）

拖拽只需产出**新的顺序**，位置由打包器派生：

```
序列 [A,B,C] + cols=12  →  pack()  →  A(0,0) B(1,0) C(2,0)   ← 提交给服务端的是这份坐标
同一个序列 + cols=4     →  pack()  →  A(0,0) B(1,0) C(2,0)   ← 窄屏只是"少几列再排一次"
```

这样做的收益：服务端"不重叠/不越界"的不变量永远满足；窄屏折行不需要额外的响应式规则；
"被挤占项顺延"是排序的天然语义，不需要螺旋搜索空位。

**一个实测约束**：承载 `use:dndzone` 的元素必须有真实布局盒。
曾经为了让「+ 添加」图块加入同一个 CSS 网格而给 `<ul>` 用了 `display: contents`，
结果 zone 的 rect 恒为 0×0，库**永远算不出落点**（拖拽能启动、元素跟着走、松手却回到原位）。
现在 `<ul>` 是真实网格，「+」按打包结果绝对定位。详见 `e2e/README.md`。

### 10.2b 拖拽与合并（svelte-dnd-action）
- 外层网格 = 一个 `dndzone` `type:'tile'`；每个文件夹 = 嵌套 `dndzone` `type:'folder-item'`（层级 type 必须不同）。
- `useCursorForDetection: true`（大图块压小目标）；`items` 保持为 `$state` 数组中的**普通对象**（避免 issue #644）。
- **合并 dwell**：`consider` 收到 `DRAGGED_ENTERED_ANOTHER` 时启动 `merge_dwell_ms` 计时器 + 目标高亮/轻抖；`DRAGGED_OVER_INDEX`/`DRAGGED_LEFT` 取消；计时到 → 执行合并（源从页面移除 + 目标文件夹 push，**同一 tick 内两侧都改**，`animate:flip` 统一 `flipDurationMs`）。
- 落点冲突：客户端把被挤占项按"最近空位"顺延（螺旋搜索），再整板提交。
- 文件夹容量 9：第 10 个被拖入时弹回原位 + toast。

### 10.3 跨页浮动拖拽层（高风险，R2 —— 已实现并验收）

```
拖动中指针进入屏幕左右 ≤60px 边缘带 ──150ms 悬停──▶ 翻到相邻页
   ├─ 无相邻页(首/尾) → 提示"已经是第一页/最后一页"，不翻
   └─ 进入 carry 模式：
        1) 记下被拖项（数据仍在源页，尚未落库）
        2) 合成一次 window mouseup，让拖拽库自己把这次拖拽收场
           （Grid 的 finalize 看到 board.carry 就只还原本地顺序、不落库）
        3) 切到目标页
        4) CarryLayer 用 pointermove 跟随指针，按"目标页序列 + 插入下标"高亮落点
        5) pointerup → pack 出目标页完整布局 → POST /api/board/move（单事务）
```

**为什么不让拖拽库直接跨页拖**：它的 zone 绑定在当前页的 DOM 上，
换页会让它缓存的元素全部失效。合成 mouseup + 自研漂浮层是这里唯一稳的路径。

**为什么落点要送"完整布局"而不是一个坐标**：插入会把目标页原有图标挤开，
只送一个 `(col,row)` 服务端无法知道其余项要挪到哪里 —— 曾经因此直接 422（坐标冲突），
图标搬不过去。现在客户端把重排后的整页布局一起提交，服务端在同一事务里既搬又重排。

### 10.4 翻页
- 点圆点：直接切页（带 200ms 淡入）。
- 滚轮：`wheel` 累加 ±120 阈值 + 300ms 节流（`prefers-reduced-motion` 时无动画）。
- 横滑：`pointerdown→move` 水平位移 >50px 且竖直位移 <30px 才判定为翻页手势（非编辑态、非拖拽中）。
- 边缘悬停翻页：见 10.3（仅拖拽中生效）。

### 10.5 搜索框
```
输入 ──▶ Fuse.search(query, {limit:20}) ──▶ 下拉列表（标注所属页名/文件夹名）
 ↑↓ 移动高亮   Enter = 用当前默认引擎搜索   Ctrl/Cmd+Enter = 打开高亮项
 点击徽标 ──▶ 引擎面板（搜索 | 6 内置 | + 自定义）
 转义/失焦 ──▶ 收起；点击站内结果 = 跳转
```
- Fuse 实例：`one instance + fuse.setCollection(links)`（`$effect`），**不要每次按键重建**。
- 测试约束：**不断言 score 数值与固定顺序**（7.5.0 改了打分与权重归一）。

### 10.6 模式（键盘）
- `/` 或 `Ctrl/Cmd+K` 聚焦搜索框；`Ctrl/Cmd+Z` 撤销；`ESC` 关模态/退出编辑态。

---

## 11. 待确认与风险

### 11.1 Q29 拖拽门控（已定：方案 A + 300ms 长按）
- **无编辑态**。触屏按住 **300ms** 进入可拖拽；鼠标即时拖。参考数值：移动端 300ms、桌面 0ms。
- 长按继续到 **800ms** 不动 → 上下文菜单（编辑 / 删除 / 移动到… / 放大为 2×2）；移动即转拖拽，不弹菜单。
- 新增：网格末尾常驻虚线 `+` 图块。删除：上下文菜单 + 桌面悬停 `×`（带确认；删文件夹时询问是否连带删除其内链接）。
- 实现参数：`svelte-dnd-action` 的 `delayTouchStart: 300`（该选项只作用于触屏）+ `touch-action: manipulation` + `-webkit-touch-callout: none`。

### 11.2 Q26 交付信息（已定：GitHub Actions → ghcr.io）
- 仓库：`https://github.com/ffdkj/my_nav`；镜像：`ghcr.io/ffdkj/my_nav`
  （`main` → `:edge`；tag `v*` → `:x.y.z` + `:latest`；`linux/amd64` + `linux/arm64` 双架构）。
- 开发机无 `gh` CLI、**Docker 守护进程不可达** → 容器构建与验证只在 CI 和 armbian 主机上进行。

### 11.3 风险登记
| id | 风险 | 影响 | 缓解 |
|---|---|---|---|
| R1 | Tailscale 边车是**新节点**，需要 auth key 且与宿主节点并存 | 阻塞部署 | 提供逐步指引；失败回退方案 = 宿主 `tailscale serve --https=8443` |
| R2 | 跨页浮动拖拽层为自研（库不支持跨 DOM 交接） | 触屏易出 bug | 单独里程碑 + 真机验收；异常一律还原不落盘 |
| R3 | 编译期依赖 arm64 构建环境 | 本机无法验证镜像 | CI 多架构构建，宿主只 pull |
| R4 | eMMC 仅剩 13.9GB | 磁盘写满 | 镜像最小化（distroless）、限制日志、`VACUUM INTO` 压缩备份 |
| R5 | sqlc 的 SQLite 支持为 **Beta** | 复杂查询可能生成失败 | 查询保持简单；必要时手写 SQL 放 `internal/db/manual.go` |
| R6 | 真机触屏行为未经实测 | 拖拽手感不达标 | 里程碑 M3 用手机实测 `delayTouchStart`/dwell 参数并调优 |
| R7 | Fuse 7.5 打分变化 | 测试脆弱 | 只断言集合成员与相对顺序意图 |

---

## 12. 里程碑（建议顺序）

| 里程碑 | 内容 | 验收 |
|---|---|---|
| **M0** | 仓库骨架、Makefile、`go.mod`、Vite 项目、Tailwind v4、CI 空跑 | 本机 `npm run dev` + `go run` 通；`svelte-check` 通过 |
| **M1** | sqlc + 迁移 + 全部表 + 整板 PUT/GET + 422/409 语义 | `go test ./...` 覆盖不变量校验（容量/重叠/嵌套/空夹） |
| **M2** | 网格渲染 + 拖拽吸附 + 编辑器面板 + 新增/删除链接 | 桌面鼠标全流程可用 |
| **M3** | 文件夹：小夹 9 缩略、模态、大夹 2×2 直接可点、合并 dwell、容量与顺延 | **手机真机**拖拽/合并验收（R6） |
| **M4** | 多页：圆点、管理抽屉、滚轮/横滑/边缘悬停翻页、跨页 carry 层 | 跨页搬图标成功率 ≥95%（R2） |
| **M5** | 图标抓取链 + 负缓存 + 上传/单色字/重新抓取 + 内容寻址 | 抓取成功/兜底/上传三条路径各有用例 |
| **M6** | 壁纸（上传/URL/轮换/兜底/每页）、设置页、引擎 CRUD | 设置项全部持久化并跨设备一致 |
| **M7** | 导出 JSON/zip、导入覆盖、pre-import 备份、备份状态与下载 | 导入后数据与导出前逐字段一致 |
| **M8** | PWA（manifest/图标/NetworkFirst/更新提示）——**组件按可选实现**，仅在 HTTPS 下生效 | 升级 HTTPS 后 iOS Safari 可"添加到主屏幕"，离线可打开 shell；http 访问下不报错、不注册 SW |
| **M9** | Dockerfile + compose + Tailscale 边车 + README + 部署清单 | 宿主 `pull` 后一条命令起，`https://nav.tailbae726.ts.net` 可用 |

---

## 13. 验收清单（Definition of Done）

**功能**
- [ ] 添加网址 → 3s 内出图标或明确兜底；重复添加同一站不重复请求
- [ ] 拖动图块可任意摆放，松手吸附，刷新后位置不变
- [ ] 拖到另一图块并**停住 500ms** → 合并为文件夹；容量 9，第 10 个弹回并提示
- [ ] 小夹外显 9 缩略图；点击开模态（壁纸虚化）；模态内可排序；拖出回到主网格
- [ ] 大夹 2×2 内 9 个图标直接可点；编辑面板可 1 格 ⇄ 2×2 切换
- [ ] 空文件夹自动消失；被挤占图标顺延到最近空位
- [ ] 多页：增删改名排序、圆点切换、滚轮/横滑/边缘悬停翻页、跨页搬图标
- [ ] 搜索：输入出下拉、回车走引擎、`Ctrl/Cmd+Enter` 开高亮项、徽标切引擎
- [ ] 壁纸：上传/URL、每页独立或跟随全局、轮换、兜底
- [ ] 导出 JSON 与 zip；导入覆盖后数据一致；`pre-import.json` 只有一份且可下载
- [ ] PWA：在 HTTPS 访问下可安装、更新有提示、离线可打开；在 http 访问下**优雅降级不报错**

**非功能**
- [ ] 冷启动首屏 <1.5s（局域网/tailnet 内）
- [ ] 拖拽稳定 60fps（中端手机 30fps 以上）
- [ ] `/api` 未知路径返回 JSON 404（不是 HTML）
- [ ] 无 `middleware.RealIP`；`/api` 写接口不做无意义 CORS
- [ ] SQLite 在 WAL 模式、`busy_timeout=5000`、`foreign_keys=1`（由 DSN 保证）
- [ ] 静态资源哈希强缓存；`index.html` `no-cache`
- [ ] 键盘可完成：翻页、开关编辑态、拖拽排序、打开链接、打开高亮搜索结果
- [ ] `prefers-reduced-motion` 下无 FLIP/淡入动画

---

## 14. 需你提供 / 需你执行

1. **GitHub 仓库地址 + 可见性**（v1 的 CI 依赖它）；`ghcr.io/<owner>/my_nav` 的 owner 归属。
2. **Tailscale auth key**（边车节点认证用；建议 ephemeral=false、reusable=false、tag 可选）。
3. 在 armbian 主机执行部署命令（MCP 无 shell 通道）——我会给出**可直接粘贴的脚本**与逐步向导。
4. 部署前待验证三项（需宿主 SSH）：`docker compose version`、`tailscale status --json` 里的 `CertDomains`、Docker Hub/`gstatic` 出网连通性。
