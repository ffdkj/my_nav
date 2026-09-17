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

用例：`smoke.mjs`（基础交互与排序）、`folders.mjs`（合并/小夹/大夹）、`pages.mjs`（多页与跨页拖拽）、
`appearance.mjs`（白天/黑夜主题、每页壁纸、换页平移动画）。
每个用例各有独立的数据目录与端口，互不污染，也能单独跑：

```bash
NAV_BASE=http://127.0.0.1:18100 node e2e/pages.mjs   # 需先自行起一个后端
```

`node debug-drag.mjs` / `node debug-slide.mjs` 是诊断脚本（拖拽 / 换页动画）。

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

