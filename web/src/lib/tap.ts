/**
 * 挡掉 `svelte-dnd-action` 在 `touchend` 上**补发**的那个 click。
 *
 * 库的源码（`handleFalseAlarm`）：
 *
 * ```js
 * // dragging initiated by touch events prevents onclick from initially firing
 * if (e.type === "touchend") {
 *   e.target.dispatchEvent(new Event("click", { bubbles: true, cancelable: true }))
 * }
 * ```
 *
 * 它假定"拖拽初始化时已经 preventDefault，原生的 click 不会来"。这条假定只在
 * **没配 `delayTouchStart`** 时成立：配了的话 `handleMouseDown` 走的是
 * `if (!useDelay) e.preventDefault()` 的**另一条分支**，触摸开始的默认行为根本没被拦，
 * 浏览器的兼容性 click 照常发。于是一次点击变成两次：库补的那个 + 原生那个。
 *
 * 实测（Chromium 触摸模拟，捕获取全部事件）：
 *
 * ```
 * pointerdown → touchstart → pointerup → touchend → click(trusted=false) → click(trusted=true)
 * ```
 *
 * 桌面看不出来（`mouseup` 不补 click）；Android 也看不出来（补发的那个是 untrusted，
 * Chromium 不给它用户手势，`target=_blank` 被弹窗拦截吃掉，最后只剩原生那一次）。
 * iPadOS Safari 会放它过去，于是**一个图标弹出两个相同页面**。
 *
 * 判据就用 `isTrusted`：真手指 / 真鼠标 / 键盘产生的 click 一定是 trusted，
 * 库补发的一定不是 —— 跟先后顺序、时间窗都无关（实测补发的那次反而**先**到，
 * 所以"按时间窗吃掉第二发"会把真正有效的那一发吃掉，反而什么都不开）。
 *
 * ⚠️ 前提：这些拖拽区必须保留 `delayTouchStart > 0`（Grid / FolderModal 都是 300ms）。
 * 若哪天改成 0，库会真的 `preventDefault` 掉原生的 click，那时**只有**补发的这一次，
 * 再挡掉就等于点击全失效。
 */
/**
 * 第二道保险：同一元素上极短时间内的**第二发真点击**也吃掉。
 * 第一道（`isTrusted`）针对的是库补发的那一发；这一道防的是"某个浏览器自己
 * 对一次 tap 发两个 trusted click"这种没实测到的情形。
 *
 * 按 `e.target` 而不是按整个拖拽区计数 —— 否则连点两个**不同**图标时，
 * 第二个会被误吞（这是真会踩到的：图标挨着点很快）。
 * 窗口取 400ms：足够合并同一手势的重复，又不至于吞掉"没反应，再点一次"。
 */
const TRUSTED_DUPLICATE_MS = 400
const lastTrustedAt = new WeakMap<EventTarget, number>()

export function blockSyntheticClicks(node: HTMLElement) {
  function onclick(e: MouseEvent) {
    if (!e.isTrusted) {
      e.preventDefault()
      return
    }
    if (e.target) {
      const now = Date.now()
      const prev = lastTrustedAt.get(e.target) ?? 0
      if (now - prev < TRUSTED_DUPLICATE_MS) {
        e.preventDefault()
        return
      }
      lastTrustedAt.set(e.target, now)
    }
  }
  // capture：抢在默认行为（导航 / 按钮激活）之前
  node.addEventListener('click', onclick, true)
  return {
    destroy() {
      node.removeEventListener('click', onclick, true)
    },
  }
}
