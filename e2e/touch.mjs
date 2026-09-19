/**
 * 触屏专属行为 —— 鼠标测不出来的那一类。
 *
 * 这两条都是真机上报出来、e2e 却全绿的 bug，所以单开一个用例：
 *
 *   1) **iPad 上点一次图标弹出两个相同标签页**。
 *      `svelte-dnd-action` 在 `touchend` 上会自己补一个 click（它假定拖拽初始化
 *      时已经 preventDefault、原生 click 不会来）。但我们配了 `delayTouchStart: 300`，
 *      于是 `handleMouseDown` 走的是 `if (!useDelay) e.preventDefault()` 的另一条
 *      分支 —— 原生默认行为没被拦，原生 click 照常发，两发都导航。
 *      桌面（mouseup 不补 click）与 Android Chrome 看不出来，所以只在 iPad 现身。
 *
 *   2) **安卓上完全滑不动**。横滑的起手判定遇到 `[data-tile]` 就 return，
 *      而手机上网格几乎铺满屏幕 —— 等于哪儿都滑不动。
 *      更深一层：`touch-action: manipulation` 允许 pan-x，浏览器横滑刚过 20 来像素
 *      就接管去平移，**发 pointercancel 而不发 pointerup**，判定拿到的位移永远不够。
 *
 * 另外把"角度判定"（±30°）钉死：30° 内算翻页、超过不算、太短不算、竖直不算。
 *
 * 手势用 CDP 的 `Input.dispatchTouchEvent` 发**真触摸事件**（不是 mouse），
 * 否则 `delayTouchStart` 那条分支根本不会走到，测了等于没测。
 */
import { createServer } from 'node:http'
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18098'

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}

async function waitSaved(page, timeout = 20_000) {
  await page.waitForFunction(() => !document.body.innerText.includes('保存中'), null, { timeout })
}

// 目标站点：离线小站，弹出来的标签页能秒开，不依赖外网
const site = createServer((_req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
  res.end('<!doctype html><title>target</title>ok')
})
await new Promise((r) => site.listen(0, '127.0.0.1', r))
const siteURL = `http://127.0.0.1:${site.address().port}/`

const browser = await chromium.launch()
// 手机尺寸 + 触摸：isMobile 让 viewport/DPR 也按真机来
const ctx = await browser.newContext({
  viewport: { width: 390, height: 844 },
  hasTouch: true,
  isMobile: true,
})
const page = await ctx.newPage()
const cdp = await ctx.newCDPSession(page)

const consoleErrors = []
page.on('console', (m) => {
  if (m.type() === 'error') consoleErrors.push(m.text())
})
page.on('pageerror', (e) => consoleErrors.push('pageerror: ' + e.message.split('\n')[0]))

/** 发一串真触摸事件（touchStart → N×touchMove → touchEnd） */
async function touchSwipe(from, to, { steps = 10, holdMs = 0 } = {}) {
  await cdp.send('Input.dispatchTouchEvent', {
    type: 'touchStart',
    touchPoints: [{ x: from.x, y: from.y }],
  })
  if (holdMs) await page.waitForTimeout(holdMs)
  for (let i = 1; i <= steps; i += 1) {
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [
        {
          x: Math.round(from.x + ((to.x - from.x) * i) / steps),
          y: Math.round(from.y + ((to.y - from.y) * i) / steps),
        },
      ],
    })
  }
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
}

const hash = () => page.evaluate(() => location.hash)
/** 关掉点图标弹出来的那些标签页，只留主页面 */
async function closePopups() {
  for (const p of ctx.pages().slice(1)) await p.close().catch(() => {})
}

try {
  await page.goto(BASE, { waitUntil: 'networkidle' })

  // 铺几个图标：既要有点击目标，也要让网格有"铺满屏幕"的真实感
  for (let i = 0; i < 3; i += 1) {
    await page.getByRole('button', { name: '添加图标' }).click()
    await page.fill('#link-url-input', siteURL)
    await page.getByPlaceholder('例如 GitHub').fill(`目标 ${i + 1}`)
    await page.getByRole('button', { name: '确定' }).click()
    await page.waitForTimeout(600)
  }
  await waitSaved(page)
  const tiles = await page.locator('[data-testid="tile-link"]').count()
  check('铺好了触屏测试用的图块', tiles === 3, `tiles=${tiles}`)

  // ---- 1) 触屏单击只开一个标签页 ----
  const first = await page.locator('[data-testid="tile-link"]').first().boundingBox()
  const tx = first.x + first.width / 2
  const ty = first.y + first.height / 2

  await page.touchscreen.tap(tx, ty)
  await page.waitForTimeout(600)
  check('触屏单击图标只开一个标签页', ctx.pages().length - 1 === 1, `新开=${ctx.pages().length - 1}`)
  await closePopups()
  await page.waitForTimeout(300)

  // ---- 2) iPad 双开回归：tap 之后再补一发 click，第二发必须被吃掉 ----
  await page.touchscreen.tap(tx, ty)
  await page.evaluate(() => {
    // 这正是 svelte-dnd-action 在 touchend 上做的事
    document
      .querySelector('[data-testid="tile-link"]')
      .dispatchEvent(new Event('click', { bubbles: true, cancelable: true }))
  })
  await page.waitForTimeout(600)
  check(
    'tap 之后补发的第二个 click 被吃掉（iPad 双开回归）',
    ctx.pages().length - 1 === 1,
    `新开=${ctx.pages().length - 1}`,
  )
  await closePopups()
  await page.waitForTimeout(300)

  // ---- 2b) 连点同一个图标：折叠成一次（第二道保险：同一元素 400ms 内的第二发真点击） ----
  await page.touchscreen.tap(tx, ty)
  await page.touchscreen.tap(tx, ty)
  await page.waitForTimeout(600)
  check('连点同一个图标只开一个', ctx.pages().length - 1 === 1, `新开=${ctx.pages().length - 1}`)
  await closePopups()
  await page.waitForTimeout(400)

  // ---- 2c) 但连点**两个不同**图标必须各开一个（去重按元素，不按整个拖拽区） ----
  const second = await page.locator('[data-testid="tile-link"]').nth(1).boundingBox()
  await page.touchscreen.tap(tx, ty)
  await page.touchscreen.tap(second.x + second.width / 2, second.y + second.height / 2)
  await page.waitForTimeout(700)
  check(
    '连点两个不同图标各开一个（没被去重误吞）',
    ctx.pages().length - 1 === 2,
    `新开=${ctx.pages().length - 1}`,
  )
  await closePopups()
  await page.waitForTimeout(400)

  // ---- 3) 两页，才能验翻页 ----
  await page.getByRole('button', { name: '页面管理' }).click()
  const manager = page.locator('div[role="dialog"][aria-label="页面管理"]')
  await manager.waitFor({ timeout: 5_000 })
  await manager.getByLabel('新页面名称').fill('Work')
  await manager.getByRole('button', { name: '新建' }).click()
  await page.waitForTimeout(900)
  const pageCount = (await (await fetch(`${BASE}/api/pages`)).json()).pages.length
  check('建好第二页', pageCount === 2, `pages=${pageCount}`)

  /** 回到第一页 → 从某块图块中心起手滑 (dx,dy) → 返回是否翻到了 Work */
  async function swipeFromTile(dx, dy, opts) {
    await page.locator('nav[aria-label="页面"] button').first().click()
    await page.waitForTimeout(700)
    const box = await page.locator('[data-testid="tile-link"]').first().boundingBox()
    const from = { x: box.x + box.width / 2, y: box.y + box.height / 2 }
    const to = { x: from.x + dx, y: from.y + dy }
    await touchSwipe(from, to, opts)
    await page.waitForTimeout(900)
    return (await hash()).includes('work')
  }

  // ---- 4) 起手落在图块上也要能滑（安卓"滑不动"就是这个） ----
  check('从图块上起手向左滑 → 翻页', await swipeFromTile(-220, 20))

  // ---- 5) 角度判定：±30° 内算横滑 ----
  // dy/dx = 100/200 = 0.5 ≈ 26.6°，落在 30° 内
  check('斜 26.6° 的滑动仍算翻页', await swipeFromTile(-200, 100))
  // 45°：明显是斜着滑，不该翻
  check('斜 45° 的滑动不翻页', !(await swipeFromTile(-200, 200)))
  // 竖直：更不能翻
  check('竖直滑动不翻页', !(await swipeFromTile(-10, -220)))
  // 太短：轻点抖动不该翻
  check('位移不足的轻滑不翻页', !(await swipeFromTile(-30, 0)))

  // ---- 6) 长按 300ms 变拖拽时，不能被当成翻页 ----
  await page.locator('nav[aria-label="页面"] button').first().click()
  await page.waitForTimeout(700)
  const lastBox = await page.locator('[data-testid="tile-link"]').last().boundingBox()
  const dragFrom = { x: lastBox.x + lastBox.width / 2, y: lastBox.y + lastBox.height / 2 }
  // 目标点夹在安全区内：太靠左会触发"拖到边缘翻页"（那是另一条路径）
  const dragTo = { x: Math.max(90, dragFrom.x - 150), y: dragFrom.y }
  await touchSwipe(dragFrom, dragTo, { steps: 8, holdMs: 450 })
  await page.waitForTimeout(800)
  check('长按变拖拽时不会被当成翻页', !(await hash()).includes('work'), `hash=${await hash()}`)

  check('浏览器控制台无 error', consoleErrors.length === 0, consoleErrors.slice(0, 2).join(' | '))
  await page.screenshot({ path: 'e2e/shot-touch.png' })
} catch (err) {
  check('执行过程未抛异常', false, err.message)
  await page.screenshot({ path: 'e2e/shot-touch-fail.png' }).catch(() => {})
} finally {
  await browser.close()
  site.close()
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
process.exit(failed.length === 0 ? 0 : 1)
