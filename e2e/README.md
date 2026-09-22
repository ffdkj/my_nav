# e2e — 真浏览器验收

```bash
make e2e          # = ./e2e/run.sh
```

它会：构建前端与后端 → 用**临时数据目录**起一个后端 → 启动 headless Chromium →
验证「初始渲染 / 通过 UI 新增链接 / 真实鼠标拖拽并持久化 / 刷新后顺序保持 / 站内搜索 / 控制台无报错」，
最后关掉后端。截图落在本目录。

为什么要它：M2 有两个 bug **编译和类型检查都发现不了**，只有真浏览器能暴露
（见下面「已知坑」）。UI 交互的验收不能靠"能构建"来证明。

`node debug-drag.mjs` 是拖拽诊断脚本（逐步打印 DOM 顺序、元素 transform、以及是否发出 PUT）。

用例：`smoke.mjs`（基础交互与排序、搜索下拉的 `Ctrl/Cmd+1…9` 与序号徽标）、`folders.mjs`（合并/小夹/大夹/大夹空白处开预览模态/夹内改图标）、
`pages.mjs`（多页、跨页拖拽、**首尾相连翻页**：首页向右滑 / 末页向左滑 / 第一页往回滚 / 末页拖到左边缘各一条）、
`icons.mjs`（图标抓取/候选/纯色/上传/重置/撑满与悬停文字）、`settings.mjs`（设置面板与图块形状）、
`transfer.mjs`（导入导出）、`pwa.mjs`、`appearance.mjs`（白天/黑夜主题、每页壁纸、蒙版与玻璃、换页平移）、
`touch.mjs`（**触屏专属**：触屏点击只算一次、横滑角度判定与起手位置）。
每个用例各有独立的数据目录与端口，互不污染，也能单独跑：

```bash
NAV_BASE=http://127.0.0.1:18100 node e2e/pages.mjs   # 需先自行起一个后端
```

⚠️ `folders.mjs` 与 `icons.mjs` 都自带**本地站点夹具**（`createServer` 提供 favicon/manifest），
私网抓取默认是关的，跑这两个用例必须给后端 `NAV_ALLOW_PRIVATE_FETCH=1`（`run.sh` 里已带）。
`touch.mjs` 用 CDP 的 `Input.dispatchTouchEvent` 发**真触摸事件**：用 mouse 驱动会绕过
`delayTouchStart` 那条分支，等于没测（见下面第 10 条坑）。

`node debug-drag.mjs` / `node debug-slide.mjs` 是诊断脚本（拖拽 / 换页动画）。
`node samesite-probe.mjs` 不是应用用例，是 **cookie SameSite 行为的证据脚本**（结论见 `docs/spec.md` §11.4、坑 11）。

### 部署后体检（只读）

```bash
NAV_BASE=http://100.70.0.29:8090 node e2e/live-check.mjs
```

它跑在**线上实例**上，因此只做只读观察（GET + 翻页只改浏览器 hash，点开大夹预览后立刻关掉、
**不点 ✎**——那会去查候选并往图标库里写字节）：版本与迁移是否落地、
页面主题是否等于服务端此刻的设置、蒙版规则（深色+照片才铺黑蒙版）、玻璃与图块形状变量、
动画时长、两方向换页是否相反、大夹空白处能否打开预览模态且夹内图标仍有 ✎、控制台有无报错。
断言写成**不变量**（例如"页面主题 == 服务端设置"）而不是固定值 —— 线上随时有人在用，
写死"当前必须是深色"这种断言只会被真人的一次点击弄红。
同理，"图标撑满图块"要按**两个合法档**逐块量（满格，或源图 <64px 时设计上的 60%），
只量第一块会在"第一块恰好是小位图"时误报。

`probe-slide.mjs` 是逐帧探针：定位"两个方向动画一样"那个 bug 时用的，
现在也保留着当线上方向回归的复核工具。

## 已知坑（都已在代码里修掉，改这里时别踩回去）

1. **传给 `use:dndzone` 的数组，每个元素顶层必须有 `id`。**
   早先把 `{item, col, row, span}` 这种包装对象传进去，库直接抛
   `missing 'id' property for item`，表现为"完全拖不动"。
   位置信息必须旁挂（按 id 查表），不能包在 item 外层。

2. **承载 `use:dndzone` 的元素必须有真实布局盒，不能用 `display: contents`。**
   早先为了把「+ 添加」图块并进同一个网格，给 `<ul>` 用了 `display: contents`，
   于是 zone 的 `getBoundingClientRect()` 恒为 0×0，库的落点判定失效。
   库的调试输出是：*"element was dropped right after it left origin but before
   entering somewhere else"*——拖拽能启动、元素跟着走，但**永远算不出落点**。
   现在 `<ul>` 是真实网格，「+」按打包器算出的下一空位绝对定位。

3. **合并的命中判定要用"拖拽开始那一刻"的布局快照。**
   库在拖拽过程中会实时重排：把 A 拖到 B 上时，A 的 shadow 占位**就落在 B 的格子里**，
   于是"指针当前所在格子"永远指向自己，永远判定不出合并目标（表现为：拖到别人身上
   悬停再久也不高亮、松手只是普通排序）。
   现在在 `DRAG_STARTED` 时用 `captureHitCells()` 冻结当时的 (col,row)，
   命中判定全部基于这份快照——对应到用户心智就是"我悬停在原来那个图标上"。
   另外 shadow 的 id 固定是 `SHADOW_PLACEHOLDER_ITEM_ID`，必须当成"自己"排除。

4. **每个用例用独立的数据目录与端口**（`run.sh` 里的 `run_case`）。
   曾经两个脚本共用一个库，`smoke` 加的 GitHub 和 `folders` 加的撞名，
   导致 `getByRole('link')` 命中 2 个元素而失败——这是测试隔离问题，不是产品 bug。

5. **取消长按必须监听 `window`，不能只绑在元素上。**
   拖拽期间库会把原元素隐藏、改用挂在 `document.body` 上的克隆，
   绑在 `<li>` 上的 `pointermove` 再也收不到事件，于是"一动就取消长按"失效——
   表现为**跨页 carry / 普通拖拽到 800ms 时凭空弹出上下文菜单**（而且菜单遮罩会挡住后续点击，
   让别的用例连带失败）。这是从截图里肉眼发现的，断言没覆盖到，现已加回归断言。

6. 排查这类问题的最快手段是打开库自带的调试模式：`setDebugMode(true)`
   （`svelte-dnd-action` 导出），它会直接把拒绝/判定理由打在 console 里。

7. **`transform` 过渡跑在合成器线程上，headless 下读不到"动画中的值"。**
   `getComputedStyle(el).transform` 与 `getBoundingClientRect()` 有时反映、有时**整段停在
   起点或终点**（实测同一个动画两个方向表现还不一样），拿它做断言就是随机失败。
   可靠的做法是断言**主线程必然看得到的东西**：inline `style` 的内容（起点 ±100% → 终点 0%）、
   `transition-property/duration`、Web Animations API（`el.getAnimations()` 的
   `CSSTransition.transitionProperty` 与 `currentTime`）、以及快照/轨道元素在不在。
   `probe-slide.mjs` 就是按这个原则写的线上实测脚本。

8. **写错表的 bug 在刷新之前看不出来。**
   "设为本页"曾经把 `wallpaper_mode/wallpaper_id` 写进**全局 settings**——而这两个键
   正好也在 settings 白名单里，于是写入**成功**、UI 立刻显示正确，只有刷新后才被服务端打回。
   同类问题（手选图标记在哪一列）也要**读服务端状态**断言，不能只看界面。

7. **transform 过渡跑在合成器线程，headless 下读不到中间值。**
   `getComputedStyle(el).transform` 与 `el.getBoundingClientRect()` 在动画播放期间
   可能**整段停在起点或终点**，而且同一个动画的两个方向表现还不一样
   （`e2e/appearance.mjs` 的 `maxShift` 就是这么来的：一个方向 0px、另一个方向 197px，
   但动画其实都正常）。
   所以断言换页动画时只能用主线程**必然**能看到的东西：inline style 的内容
   （`translateX(±100%)` → `translateX(0%)`）、`transition-property/duration`、
   快照与壁纸轨道在不在、以及固定 UI 的 rect（它们没有 transform，读数稳定）。
   想看逐帧真相用 `node e2e/debug-slide.mjs`（会打印 rect/computed/inline 三列，便于对照）。

8. **"写错表却写成功了"这类 bug 只有跨一次刷新才暴露。**
   `appearance.mjs` 的核心回归是：把 `wallpaper_mode/wallpaper_id` 写进全局 `settings`
   也能"成功"（服务端白名单里有这两个键），点完当下看起来完全正常，
   只有刷新后才被服务端打回 `global`。因此这类用例必须同时断言
   **服务端数据**（`GET /api/pages/`）与**刷新后的画面**，只看当前 DOM 会漏。

9. **"垫在下面的整块热区"不能用默认点击位置去点，也不能靠点击验证层级。**
   大夹的空白热区是一个 `inset-0` 的按钮，**中心被九宫格图标盖住**——
   Playwright 的可操作性检查会判定"元素被 `<a>` 拦截"而一直重试到超时，
   必须 `click({ position: { x: 4, y: 4 } })` 点内边距；
   而"图标自己还能点、没被热区抢走"这种层级关系，用
   `document.elementFromPoint()` 在**图标中心**与**内边距**各探一次最省事
   （点一次图标就跳走了，headless 下没法断言）。
   热区提示的显隐也不要读 `box-shadow`/`background-color`：那些是过渡属性，
   headless 下会卡在起点；读 `--tw-ring-color` 这类**不参与过渡的自定义属性**才稳。

10. **触屏的东西必须用真触摸事件测，而且别信"鼠标版通过了"。**
    `touch.mjs` 里两处都是鼠标测不出来的：
    - `svelte-dnd-action` 只在 `touchend` 上补发 click，`mouseup` 不补 ——
      用 `page.mouse` 驱动永远看不到"一次 tap 两次导航"。
      用 CDP `Input.dispatchTouchEvent`（`hasTouch: true` 的 context）才有真触摸。
      判"重复点击"要看 `isTrusted`：补发的那发是 `trusted=false`，
      而且它**先于**原生那发到达（实测顺序 `touchend → click(false) → click(true)`），
      所以"按时间窗吃掉第二发"会把真正有效的那发吃掉 —— 这类断言要连**开几个标签页**一起数。
    - 横滑的成败还取决于 CSS `touch-action`：`manipulation` 允许 pan-x，
      浏览器横滑过 20 来像素就接管平移，**发 `pointercancel` 而不发 `pointerup`**。
      诊断时把 `pointerdown/pointermove/pointerup/pointercancel/touchmove/touchend`
      全捕获取打一份日志，一眼就能看出断在哪一步（本仓库诊断脚本：`.smoke/debug-swipe-touch.mjs`）。
    另外：**索引页面的产物是编进 Go 二进制的**，改完 `web/` 只跑 `npm run build`
    而后端还是旧 `bin/nav` 时，浏览器拿到的仍是旧 CSS/JS ——
    改动像是"没生效"。必须 `go build` 重编（`run.sh` 会一起做）。

11. **curl 验证不了 cookie 的 SameSite 行为，别拿它下结论。**
    `curl -c/-b` 的 cookie jar **完全不实现 SameSite**：同一个"跨站点点进去"的场景，
    curl 永远给你 200，浏览器却会因为 `SameSite=Strict` 把 cookie 扣下而 401。
    这类问题只能用真浏览器验（本仓库证据脚本：`e2e/samesite-probe.mjs`，
    六个场景 + `SAMESITE=Strict|Lax` 可切，跑法见文件头注释）。
    配套的两个事实：① `http://127.0.0.1` 与 `http://localhost` 是**不同的 site**
    （IP 与主机名各自成 site，端口不参与判定），要造"跨 site"就得换主机名；
    ② **同 site 跨 origin**（`a.example.com` → `b.example.com`）Strict cookie 照发 ——
    所以"把服务挂到同一个 registrable domain 上"是这类问题的部署级解法。

