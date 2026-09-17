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

3. 排查这类问题的最快手段是打开库自带的调试模式：`setDebugMode(true)`
   （`svelte-dnd-action` 导出），它会直接把拒绝/判定理由打在 console 里。
