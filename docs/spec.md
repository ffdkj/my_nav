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
| 5 | 翻页 | 点圆点 / 滚轮 / 手势横滑（**左滑=下一页，右滑=上一页**；起手位置不限，判定按**角度 ≤30°**，见决策 39）/ **拖拽悬停屏幕左右边缘**（阈值 `page_flip_edge_ms`，默认 150ms，仅拖拽中生效）；**三条入口一律首尾相连**（决策 40） |
| 6 | 文件夹 | 单层不嵌套；容量 **9**；空夹自动删；小夹 1 格；大夹 2×2 格 |
| 7 | 小夹外观 | **固定 9 个缩略图**（不降级）；点击整夹 → 模态（壁纸虚化、可内部拖拽排序、拖出即回主网格） |
| 8 | 大夹外观 | 内部 **9 个图标直接可点**；**空白处点开与小夹同一个预览模态**（决策 36）；拖拽管理照旧；编辑面板开关切换 1 格 ⇄ 2×2 |
| 9 | 占位冲突 | 被挤占项**顺延到最近空位**；文件夹不因内容少而自动缩回 |
| 10 | 合并触发 | 悬停在目标图块上达 **500ms**（设置可调）后**武装**合并（目标放大高亮），**松手时执行**；中途移开即取消。不在拖拽中途改结构，避免拖拽库状态错乱 |
| 11 | 跨页搬图标 | **边缘悬停翻页 + 拖拽继续**：合成一次 `mouseup` 让拖拽库体面收场 → 切页 → 自研漂浮层接管指针 → 松手走**原子端点** `POST /api/board/move`（它接收**目标页搬完后的完整布局**，在单事务里既搬又重排目标页其余项）。高风险项 R2，已用真浏览器验收 |
| 12 | 图标抓取 | 添加时**后端同步抓取**；链路 `faviconV2` → DuckDuckGo `ip3` → 自建 HTML 发现 → 纯色文字兜底 |
| 13 | 图标存储 | **原样字节**存盘（内容寻址），不转码；DB 记 MIME/尺寸/来源 |
| 14 | 图标兜底 | 编辑面板：重新抓取 / 上传本地（≤512KB，缩到 256×256）/ 纯色文字（10 预设色 + 取色器）/ 打开原站 / 重置为标准 favicon |
| 15 | 搜索框 | 输入即下拉**全局**站内匹配；**回车 = 当前引擎搜索**；`↑↓` 高亮 + **`Ctrl/Cmd+Enter` 打开高亮项**；徽标点击弹引擎面板；**`Ctrl/Cmd+1…9` 直接打开第 N 条匹配**（行首带序号徽标，决策 41） |
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
| 28 | 主题 | `settings.theme ∈ {light, dark, auto}`（服务端为事实源）→ 切 `<html class="dark">`；组件只用语义 token，**不写 dark: 变体**；`localStorage` 仅作首屏缓存防闪 |
| 29 | 每页壁纸 | 写 **`pages.wallpaper_mode/wallpaper_id`**（`PATCH /api/pages/{id}`），**绝不写全局 settings**；实测把这两字段写进 settings 会造成"改本页却动了全局，刷新又被改回跟随全局" |
| 30 | 换页动画 | 平移 260ms：**图标层**推入推出（旧页用换页前的 DOM 快照出场）；**壁纸层**仅在新旧页壁纸不同时平移（相同时纹丝不动）；**固定 UI**（搜索栏/主题按钮/设置/页码圆点）不参与；`prefers-reduced-motion` 直接跳过 |
| 31 | 蒙版与玻璃 | **浅色主题不再压白蒙版**（实测"白天那层白"会把壁纸整张洗掉）；照片上的可读性改由**玻璃 token** 提供：`:root[data-photo='1']` 把 `--c-glass/--c-glass-hover/--c-glass-ring/--c-dot` 翻成白色半透明。深色仍压黑蒙版（`--wallpaper-scrim` 只存在于 `.dark`）。`<html data-photo>` 由 `WallpaperLayer` 维护 |
| 32 | 换页动画修订 | 平移 **300ms** + `cubic-bezier(.32,.72,0,1)`；层用 `translate3d` 强制合成层；**壁纸轨道的方向必须跟着图标**：`dir=+1` 走 `0% → -50%`（向左），`dir=-1` 走 `-50% → 0%`（向右）—— 早先两个方向都写死 `0% → -50%`，导致"上一页"先闪出新壁纸、再滑向旧壁纸、收尾又跳回去；动画期间在 `<html>` 挂 `data-page-sliding`，**临时关掉玻璃面的 `backdrop-filter`**（移动层上每帧重算模糊是掉帧主因） |
| 33 | 图块形态 | 图标**撑满整块**（`object-contain`，容器给中性玻璃底），标题改为**悬停/聚焦时从底部浮出的压条**；`icon_w < 64` 的小位图只放大到 60%（避免糊）。形状预设 **全局设置 `settings.tile_shape ∈ {rounded, circle, squircle, square}`**（白名单 + 值校验），由 App 写进 `--radius-tile` / `--radius-tile-lg`；大文件块单独一档（圆形预设下用 26%），避免 2×2 变正圆把九宫格四角切掉 |
| 34 | 图标候选 | `POST /api/icons/candidates {url}` 抓**多张**候选（站点 `<link icon>` 全家 + **manifest icons[]** + `/favicon.ico` + Google `size=256` + DDG），字节**直接落内容寻址仓库**并回传 `/icons/...`；排序 = 质量档（矢量 / ≥180 且透明）> 来源可靠度 > 尺寸，**先排序再去重**（同图多来源时让分数高的来源活下来）。选中后 `POST /api/links/{id}/icon/pick {icon_path, remote_url}` 只改指针并记 `icon_picked_url`。`icon_source` 的 CHECK 约束不动（SQLite 改约束要重建表） |
| 35 | CI 触发 | `test` 留在 push/PR（便宜），**镜像只在 `v*` tag 与手动 `workflow_dispatch` 构建**；`paths-ignore` 跳过纯文档改动（GitHub 明确 path filters 不作用于 tag push） |
| 36 | 大夹可点区域 | 2×2 大夹**内部 9 个图标仍然直接跳转**（Q15 决策 3 不变），而**空白处（内边距 + 图标之间的缝）点开与小夹同一个预览模态**——两处复用同一个 `FolderModal`。实现上是一个 `inset-0` 的按钮**垫在图标层下面**（不是包在外面：`<a>` 嵌进 `<button>` 是非法 HTML）：CSS 的 `:hover` 只沿祖先链传播，所以悬停图标不会点亮"展开"提示，只有真指在空白处才亮（`--tw-ring-color` 由 transparent 变 accent，e2e 断言的就是这个属性） |
| 37 | 夹内改图标 | 预览模态里每个夹内图标左上角 ✎ → **复用普通图块的编辑对话框**（同一份 `editing` 状态 + 同一份候选卡片逻辑；`LinkDialog` 抬到 `z-[60]` 压住模态）。选完确定：图标当场换掉、**模态保持打开**（连改几个不用重新展开）；"移出到主网格" ↗ 在右上角。鼠标悬停才浮出，`pointer-coarse` 下常显 |
| 38 | 触屏点击只算一次 | `svelte-dnd-action` 会在 `touchend` 上**补发一个 click**（源码 `handleFalseAlarm`），它假定"初始化拖拽时已 `preventDefault`、原生 click 不会来"；但我们配了 `delayTouchStart: 300`，`handleMouseDown` 走的是 `if (!useDelay) e.preventDefault()` 的**另一条分支**，原生 click 照发 → 一次 tap 两次导航，**iPad 上一个图标弹出两个相同页面**（桌面 `mouseup` 不补 click、Android 那次补发因 untrusted 拿不到用户手势被弹窗拦截，所以都看不出来）。实测事件的固定顺序是 `… touchend → click(trusted=false) → click(trusted=true)`。修法：在两个拖拽区 `<ul>` 上挂 `use:blockSyntheticClicks`，**capture 阶段 `preventDefault` 掉 `!isTrusted` 的那一发**，再叠一道"同一 `e.target` 400ms 内第二发真点击也吃掉"。**判据必须用 `isTrusted` 而不是时间窗**：补发的那发反而**先**到，按时间窗吃第二发会把真正有效的那发吃掉，结果一个都打不开（已实测）。前提是这两个区必须保持 `delayTouchStart > 0` |
| 39 | 触屏横滑判定 | 两处修订：①**起手不再排除 `[data-tile]`**（手机上网格铺满屏幕，那条 `closest('[data-tile]') → return` 等于哪儿都滑不动，安卓"没反应"就是这个）；②判定从"纵向位移 <40px"的绝对阈值改成**角度**：与水平夹角 ≤30° 即算翻页（`|dy| ≤ |dx|·tan30°`，地板 `|dx| ≥ 50px` 防轻点误翻）。只量"第二处"是不够的 —— `touch-action: manipulation` 允许 pan-x，浏览器横滑过 20 来像素就接管平移并**发 `pointercancel` 而不发 `pointerup`**（实测 `pointerdown@52 → pointermove@30 → pointercancel@0,0`，判定只拿到 22px），所以 `<main>` 与 `[data-tile]` 的 `touch-action` 收成 `pan-y`（`<main>` 保留 `pinch-zoom`），横向明确留给 JS，纵向照旧原生滚动、仍是合成器快路径。`pointerup`/`pointercancel` 都走同一个收尾判定（浏览器接管后不会再发 `pointerup`） |
| 40 | 首尾相连翻页 | 页面成环：`adjacentPage(dir)` 两端接环（`pages[(i+dir+n)%n]`），**末页继续向后 = 首页，首页继续向前 = 末页**。三条入口一起循环（横滑 / 滚轮 / 拖到屏幕边缘），因为它们都走同一个 `adjacentPage`。只有一页时仍返回 `undefined`（"没有邻页"与"绕回自己"是两回事，否则单击一下会白重载一次当前页）。**动画方向跟手势**：环上那一跳的页码差是负的，按页码差算新页会从反方向飞进来（露馅成一记回跳），所以 `selectPage(id, dirOverride?)` 由手势路径（`flip` / `beginCarry`）传方向；点圆点与页面管理跳页没有手势，仍按页码差。到边提示（"已经是最后一页/第一页"）随之成为死代码被删除 |
| 41 | 搜索结果的 Ctrl/Cmd+数字 | 搜索框下拉展开且有焦点时，**`Ctrl/Cmd+1…9` 直接打开第 N 条站内匹配**，行首显示 `1…9` 序号徽标；结果上限 8 → **9**（凑满九个槽位），列表 `max-h-[50vh]` 自己滚、底部提示行常驻。判据用 `e.code`（`^(Digit\|Numpad)[1-9]$`）而不是 `e.key`（按住 Ctrl 时某些布局下 `e.key` 不是数字）；小键盘同样计数。**越界不抢键**（只有 3 条时按 Ctrl+5 不处理也不 `preventDefault`，否则会莫名禁掉浏览器本来的行为）。`preventDefault` 是为了抢在浏览器"Ctrl+数字切标签页"之前 —— **抢不抢得赢由浏览器决定**（该键是否先发给网页各浏览器不一），装成 PWA 时没有标签页、冲突不存在。`Ctrl/Cmd+Enter` 打开高亮项保留 |

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
| PATCH | `/api/links/{id}` | 改标题/URL/打开方式/单色字字段（改 URL 只在 `icon_source=auto` **且**没有手选（`icon_picked_url` 为空）时才重置图标） |
| DELETE | `/api/links/{id}` | 删链接（同时清 placement） |
| POST | `/api/links/{id}/icon/refetch` | 手动重新抓取（忽略负缓存） |
| POST | `/api/links/{id}/icon/upload` | multipart 上传本地图标（≤512KB，缩 256×256） |
| POST | `/api/links/{id}/icon/monogram` | 切纯色文字（`{text,color,font_size}`） |
| POST | `/api/links/{id}/icon/reset` | 重置为标准 favicon（`icon_source=auto` + 重新抓取，同时清掉手选标记） |
| POST | `/api/links/{id}/icon/pick` | 采用候选里的一张：`{icon_path, remote_url}` → 只改指针 + 记 `icon_picked_url`（字节早已在仓库里） |
| POST | `/api/icons/candidates` | `{url}` → `{url, candidates[]}`；**不需要链接已存在**（新增对话框输入网址即可查），最多 6 张，同一 URL 60s 内不重复抓 |
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
| 3 | 自建发现：GET 站点 HTML → 解析 `<link rel="icon"\|shortcut icon\|apple-touch-icon\|mask-icon>`（按研究里的优先级，href 依 `<base href>` 解析）→ 取最大/最合适；再兜 `/favicon.ico` | 200 + 可解析图片 | 无 link 且 `/favicon.ico` 非 200 | 1.5s（实现：外部两步各限 700ms，剩下留给它） |
| 4 | 失败 → `icon_status=miss`，前端直接打开**纯色文字**兜底面板 | — | — | — |

- 第 1 步返回 <64px 时才跑第 3 步尝试升级（避免无谓请求）。
- `s2/favicons` **不直接用**：它只是 301 跳板、body 是 HTML，等价于 `faviconV2` 多一跳。
- **负结果缓存**：`icon_status=miss` + `icon_checked_at`，30 天内不自动重试（手动"重新抓取"可绕过）。
- 自定义 `User-Agent`（如 `my_nav/1.0 (+personal)`）；全局并发信号量 **4**。

### 7.1b 候选清单（`POST /api/icons/candidates`，预算 8s）

自动链路（上表）只挑**一张**；候选接口是"用户主动要挑"，所以：

- 来源：站点自述的全部合格候选 + **manifest `icons[]`**（现代站点往往只在这里声明 192/512）+ `/favicon.ico` + Google `size=256` + DDG；并发抓取（4），并发去重。
- 元数据：`alpha`（透明底判定，PNG/WebP/GIF 抽样扫 alpha 通道；JPEG 必为 false；SVG/ICO 为 null）与 ICO 的**头部尺寸**（读 ICONDIR，不解码像素）。
- 排序：质量档 3 = 矢量 / ≥180 且透明；2 = ≥180；1 = 其余。同档比来源可靠度（apple-touch ≈ manifest > link-icon > google > ddg > mask-icon ≈ favicon.ico），再比尺寸。
- 同图去重（sha256）在**排序之后**做，保证留下的是"来路更好"的那一份。
- 结果直接落盘（内容寻址），前端拿 `icon_path` 当缩略图；选中时只回传路径。
- 同一 URL 60s 内不重复抓（内存缓存，上限 64 条，个人规模足够）。

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
  ⚠️ 这两个字段在 **`pages` 行**上，改它们要走 `PATCH /api/pages/{id}`。曾经前端把它们写进了
  全局 `settings`（键名还恰好在服务端白名单里，所以**静默成功**）：结果是"设为本页"改了全局壁纸、
  本页的值只活在内存里，刷新即被服务端打回 `global`。这类"写错表还写成功了"的 bug 只有
  跨一次刷新才暴露，e2e 必须断言**服务端数据 + 刷新后的画面**，不能只看当前 DOM。
- **壁纸列表在 boot 阶段就要拉取**（与首页 board 并行）。`activeWallpaper` 是按 id 在
  `board.wallpapers` 里查表，表没加载就返回 `undefined` → 壁纸层直接掉到兜底底色 + 蒙版，
  表现为"设好的壁纸一刷新就没了、整页糊着一层暗"。
- 兜底：纯色/渐变可配（`wallpaper_fallback`）；**仍是出厂默认值 `#0b1220` 时跟随主题底色**，
  用户自己挑过颜色才按用户的选择。图片 404 时自动降级。
- 渲染：固定定位图层 + `background-attachment: fixed` 语义；**有照片时**才压蒙版
  （`--wallpaper-scrim` 跟随主题：深色压黑 / 浅色压白，保证两种主题下图标文字都可读）；
  没有照片时不铺蒙版 —— 兜底底色本就该是干净的纯色，再压一层暗渐变就是"整页蒙了一层黑"
  的元凶（连白天主题都像蒙了层黑）。`prefers-reduced-motion` 时关闭过渡。

---

## 9. 前端架构要点

- **状态**：`*.svelte.ts` 里的 class 单例（`board`、`edit`、`dnd`、`settings`）；组件通过 `$props()` 接收，不用 store API。
- **乐观更新**：拖拽/合并在内存立即生效，PUT board 失败则回滚 + toast。
- **撤销栈**：内存保留最近 20 步（同一页 board 快照）→ `Ctrl/Cmd+Z`。
- **设计令牌**：Tailwind v4 `@theme` 里的颜色值是 **`var()` 间接层**（`--color-surface-800: var(--c-surface-800)`），
  `--c-*` 在 `:root`（浅色）与 `.dark`（深色）里给实际颜色。于是 `bg-surface-800`、`text-fg/60`
  这类**同一批类名**在两种主题下自动取到不同颜色 —— 组件里**一个 `dark:` 变体都不写**。
  前景色统一用 `fg` 语义 token（深色下近白、浅色下近黑），`white/N` 只允许出现在**彩色底**上
  （主色按钮/单色字圆片/toast），因为那里要的确实是白色。
  压在**壁纸之上**的固定 UI（图块/文件夹/搜索栏/头部按钮/页码圆点）另有一组 `glass` token
  （`bg-glass` / `ring-glass-ring` / `hover:bg-glass-hover` / `bg-dot`），它们的取值维度是
  "主题 × 画面里有没有照片"：浅色无照片 = 深色 10% 淡染，浅色有照片 = 白色 55%（照片上不压蒙版的
  代价就用这里补回来），深色两档一致。玻璃面的模糊统一走 `.frosted` 类，好让动画期间一处关掉。
  图块圆角读 `--radius-tile` / `--radius-tile-lg`（由 `settings.tile_shape` 写进 `<html>` 内联样式）。
  `.dark` 与 `:root` 特异性相同，靠源码顺序决胜，所以覆盖块必须写在 `@theme` 之后。
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

### 9.2 换页平移动画（哪些层动、哪些层不动）

| 层 | 动？ | 怎么实现 |
|---|---|---|
| 图标网格 | 动 | 换页**前**把当前网格 `cloneNode(true)` 成快照放进"出场层"，新网格从屏幕外推进来 |
| 壁纸 | **仅当新旧页壁纸不同** | 铺两层轨道（`w-[200%]`），整体平移 `-50%`；壁纸相同时保持单层不动（同一张图平移看不出来，反而露接缝） |
| 搜索栏 / 主题按钮 / 设置入口 / 页码圆点 | **不动** | 它们在动画层之外，没有 transform |

三个实现要点：

1. **旧内容用 DOM 快照，不再渲染一份 Svelte 列表。** `svelte-dnd-action` 的 zone 按 item id 认元素，
   两份同 id 的列表会互相干扰；而快照本来就该静止、不可交互 —— 克隆天然如此（顺手加 `inert`，
   否则克隆里的 `<a href>` 还能被 Tab 聚焦）。
2. **方向按页码差算**（不是"永远往左"），所以点圆点跳页也有合理方向；
   动画状态放在 store 里（`transition` / `animating`），网格层与壁纸层读同一份，
   两层的位移天然同步。
3. **两帧技巧**：先渲染起点（`translateX(±100%)`）并让浏览器上屏，下一帧再改成终点（`0%`），
   CSS transition 才有差值可过渡。同帧改两次会被合并成一次，动画不会触发。

`prefers-reduced-motion` 时直接结束过渡（不铺快照）；时长唯一事实源是 app.css 的 `--page-slide-ms`，
store 里的收尾定时读它（注意压缩后是 `.26s`，要按单位换算）。

> 写 e2e 断言时注意：transform 过渡跑在**合成器线程**，headless 下
> `getComputedStyle().transform` 与 `getBoundingClientRect()` 可能整段停在起点或终点
> （实测两个方向表现还不一样）。断言要用主线程必然可见的状态：inline style 的内容、
> `transition-property/duration`、快照与轨道是否存在、以及固定 UI 的 rect（它们没有 transform）。

## 10. 交互状态机

### 10.1 页面级（无编辑态；Q29 已定）
```
触屏:  手指按下 ──300ms 未移动──▶ 进入可拖拽（图块抬起放大反馈）
                       │
                       ├─ 移动 → 拖拽 / 合并 / 拖到边缘翻页
                       └─ 继续按住到 800ms 仍不动 → 弹出上下文菜单（编辑 / 删除 / 移动到… / 放大为 2×2）
鼠标:  按下即拖（`delayTouchStart` 只作用于触屏，符合 svelte-dnd-action 的命名与语义）

常驻交互: 点击链接=跳转; 点击小夹=模态; 点击大夹内图标=跳转; 点击大夹空白处=同一个预览模态
        模态里每个图标 ✎=改这个图标的图标（复用普通图块的编辑对话框）; ↗=移出到主网格
        滚轮/横滑=翻页（横滑起手位置不限，与水平夹角 ≤30° 即算横滑）
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
- 点圆点：直接切页（带 200ms 淡入），动画方向按页码差。
- 滚轮：`wheel` 累加 ±120 阈值 + 400ms 节流（`prefers-reduced-motion` 时无动画）。
- 横滑：`pointerdown→move` 判定按**角度**（与水平夹角 ≤30°，地板 `|dx| ≥ 50px`，见决策 39），方向就是手势方向。
- 边缘悬停翻页：见 10.3（仅拖拽中生效）。
- **首尾相连（决策 40）**：末页继续向后 = 首页，首页继续向前 = 末页；三条入口共用 `adjacentPage`，所以一起成环。环上那一跳的动画方向**跟手势**（`selectPage(id, dirOverride?)`），否则会露馅成一记回跳。

### 10.5 搜索框
```
输入 ──▶ Fuse.search(query, {limit:9}) ──▶ 下拉列表（行首 1…9 序号 + 所属页名）
 ↑↓ 移动高亮   Enter = 用当前默认引擎搜索   Ctrl/Cmd+Enter = 打开高亮项
 Ctrl/Cmd+1…9 = 直接打开第 N 条匹配（越界不抢键，见决策 41）
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

### 11.4 已知限制：从 my_nav 点 DSH 的带 token 链接会 401（不改代码，只记录）

**现象**：图块 URL 存的是 `https://debian-nayun.tailbae726.ts.net/?token=…`，点开后地址栏变成不带 token 的 `/`，页面是 DSH 自己的一行 401。

**根因**（真浏览器复现，非推测）：DSH 的会话 cookie **硬编码 `SameSite=Strict`**（`@deepseek-ai/dsh-client-connection/lib/index.js` 里的 `sessionCookie()`），而 **Strict cookie 在"跨 site 发起"的导航上不会被发送** —— 从 my_nav 的源点过去正属于这一类。于是 `GET /?token=…` 先 303 + `Set-Cookie`（cookie 确实种上了），紧接着那次 `/` 请求**不带 cookie** → 401。token 从地址栏消失是 DSH 303 跳转的**既定设计**（不让 token 留在历史里），不是被裁掉。

**my_nav 侧无关**：URL 原样存库（`/api/links` 可查）、图块 `<a href={link.url}>` 原样打开，全程没有任何截断。

**证据脚本**：`e2e/samesite-probe.mjs`（两个站点 + 真 Chromium，打印六种场景下浏览器到底带不带这条 cookie；`SAMESITE=Strict|Lax` 可切）。
⚠️ 用 `curl -c/-b` 验证同一个 URL 会得到 200 —— **curl 的 cookie jar 不实现 SameSite**，它天生看不见这个坑，别拿它当证据。

**可用的绕法**（都不改 DSH）：点开后**按一下刷新**（刷新是"浏览器发起"，cookie 会带上）；或用书签/地址栏进。
**真修的两条路**（都动 DSH，本仓库未采用）：① `SameSite` 改 `Lax`；② token 命中时直接 200 返回应用、不 303。
**第三条不动 DSH 的部署路**（本次也未采用）：把 my_nav 挂到同一 registrable domain 的 HTTPS 上（`tailscale serve --https=8443 http://127.0.0.1:8090`），此时导航是 same-site，Strict cookie 照常发送 —— 已在证据脚本的 `[F]` 场景验证。

---

## 12. 里程碑（建议顺序）

| 里程碑 | 内容 | 验收 |
|---|---|---|
| **M0** | 仓库骨架、Makefile、`go.mod`、Vite 项目、Tailwind v4、CI 空跑 | 本机 `npm run dev` + `go run` 通；`svelte-check` 通过 |
| **M1** | sqlc + 迁移 + 全部表 + 整板 PUT/GET + 422/409 语义 | `go test ./...` 覆盖不变量校验（容量/重叠/嵌套/空夹） |
| **M2** | 网格渲染 + 拖拽吸附 + 编辑器面板 + 新增/删除链接 | 桌面鼠标全流程可用 |
| **M3** | 文件夹：小夹 9 缩略、模态、大夹 2×2 直接可点（空白处开同一个预览模态、夹内可改图标）、合并 dwell、容量与顺延 | **手机真机**拖拽/合并验收（R6） |
| **M4** | 多页：圆点、管理抽屉、滚轮/横滑/边缘悬停翻页、跨页 carry 层 | 跨页搬图标成功率 ≥95%（R2） |
| **M5** | 图标抓取链 + 负缓存 + 上传/单色字/重新抓取 + 内容寻址 | 抓取成功/兜底/上传三条路径各有用例 |
| **M6** | 壁纸（上传/URL/轮换/兜底/每页）、设置页、引擎 CRUD | 设置项全部持久化并跨设备一致 |
| **M7** | 导出 JSON/zip、导入覆盖、pre-import 备份、备份状态与下载 | 导入后数据与导出前逐字段一致 |
| **M8** | PWA（manifest/图标/NetworkFirst/更新提示）——**已完成**：`registerType: prompt` + 应用侧写穿缓存；非安全上下文下静默降级 | e2e 双路径验收：安全上下文（127.0.0.1）下 SW 接管 + 离线可打开且数据来自缓存；局域网 IP 下不注册、不报错、应用照常可用 |
| **M9** | Dockerfile + compose + Tailscale 边车 + README + 部署清单 | 宿主 `pull` 后一条命令起，`https://nav.tailbae726.ts.net` 可用 |
| **M10** | 外观修缺陷：白天/黑夜真正生效（语义色板 + 蒙版跟随主题）、每页壁纸写对表、换页平移动画 | `e2e/appearance.mjs` 30 项全绿：切主题后组件底色一起翻、刷新后主题/本页壁纸都保住、动画只动该动的层 |
| **M11** | 遮罩/动画打磨（浅色去白蒙版 + 玻璃 token、壁纸轨道方向修正、300ms + 合成层 + 动画期关模糊）、图块撑满 + 形状预设、**图标候选**（多源候选 + 手选 + manifest/透明/ICO 尺寸 + 迁移 002）、CI 降频 | `e2e` 全绿且新增：候选列表与 pick 生效、撑满与悬停文字、形状预设落到 CSS 变量、两方向轨道相反、浅色无蒙版且玻璃翻白 |

---

## 13. 验收清单（Definition of Done）

**功能**
- [ ] 添加网址 → 3s 内出图标或明确兜底；重复添加同一站不重复请求
- [ ] 拖动图块可任意摆放，松手吸附，刷新后位置不变
- [ ] 拖到另一图块并**停住 500ms** → 合并为文件夹；容量 9，第 10 个弹回并提示
- [ ] 小夹外显 9 缩略图；点击开模态（壁纸虚化）；模态内可排序；拖出回到主网格
- [ ] 大夹 2×2 内 9 个图标直接可点；空白处点开与小夹同一个预览模态；编辑面板可 1 格 ⇄ 2×2 切换
- [ ] 预览模态里每个图标都能 ✎ 改图标（复用普通图块的编辑对话框，含候选卡片），改完模态不关
- [ ] 空文件夹自动消失；被挤占图标顺延到最近空位
- [ ] 多页：增删改名排序、圆点切换、滚轮/横滑/边缘悬停翻页、跨页搬图标；**两端首尾相连**（三条入口都成环，环上那一跳的动画方向跟手势）
- [ ] 搜索：输入出下拉、回车走引擎、`Ctrl/Cmd+Enter` 开高亮项、**`Ctrl/Cmd+1…9` 开第 N 条**（行首有序号徽标）、徽标切引擎
- 壁纸：上传/URL、每页独立或跟随全局、轮换、兜底
- [ ] 白天/黑夜：一键切换（含「跟随系统」），**组件底色一起翻**，刷新后保持；壁纸蒙版深浅色各一套，无壁纸时不铺蒙版
- [ ] 每页壁纸："设为本页"只改本页、不动全局；刷新后仍是本页那张；删除壁纸后引用它的页面退回跟随全局
- [ ] 换页：图标平移、壁纸仅在换了壁纸时平移、搜索栏与设置按钮纹丝不动
- [ ] 换页的两个方向必须**相反**（上一个页面向右、下一个页面向左），壁纸层跟着同向；动画期间固定 UI 不参与、掉帧不明显（动画期关掉玻璃模糊）
- [ ] 图块：图标撑满整块、标题悬停/聚焦浮出；形状预设（圆角/圆形/超椭圆/直角）在设置里一键切换并持久化
- [ ] 图标候选：输入网址 600ms 后列出多张候选（含出处与尺寸），选中并确定后 `icon_picked_url` 落库；"重新抓取"覆盖手选、改 URL 不抹掉手选
- [ ] 浅色主题下照片上**没有白蒙版**，图块/搜索栏/圆点自动换成白色玻璃；深色下仍压黑蒙版
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
