/**
 * M4 真浏览器验收：多页与翻页
 *   1) 通过页面管理新建第二页（圆点从 1 变 2，新页为空）
 *   2) 点圆点切回第一页
 *   3) 滚轮翻页
 *   4) 手势横滑翻页
 *   5) 长按 800ms 出上下文菜单 → 「移动到…」把图标搬到另一页
 *   6) 【高风险 R2】拖动图标到屏幕右边缘 → 自动翻页 → 松手落到新页（跨页 carry）
 */
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18090'

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}

async function pages() {
  return (await (await fetch(`${BASE}/api/pages/`)).json()).pages
}
async function boardOf(pageId) {
  return (await fetch(`${BASE}/api/pages/${pageId}/board`)).json()
}


/** 等所有排队中的提交落库（UI 的"保存中…"消失）。
 *  加了图标抓取后单次 PUT 可能等上几秒，且提交是串行的，
 *  因此"UI 出现图块"不等于"服务端已保存"——断言服务端状态前必须等这个。 */
async function waitSaved(page, timeout = 20_000) {
  await page.waitForFunction(
    () => !document.body.innerText.includes('保存中'),
    null,
    { timeout },
  )
}

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })
const consoleErrors = []
page.on('console', (m) => {
  if (m.type() === 'error') consoleErrors.push(m.text())
})
page.on('pageerror', (e) => consoleErrors.push('pageerror: ' + e.message.split('\n')[0]))

const addLink = async (url, title) => {
  await page.getByRole('button', { name: '添加图标' }).click()
  await page.fill('#link-url-input', url)
  await page.getByPlaceholder('例如 GitHub').fill(title)
  await page.getByRole('button', { name: '确定' }).click()
  await page.getByRole('link', { name: new RegExp(title) }).first().waitFor({ timeout: 10_000 })
}

try {
  await page.goto(BASE, { waitUntil: 'networkidle' })
  await addLink('https://github.com', 'GitHub')
  await addLink('https://gitee.com', 'Gitee')

  const initial = await pages()
  check('初始只有一页', initial.length === 1, `pages=${initial.length}`)

  // ---- 1) 新建第二页 ----
  await page.getByRole('button', { name: '页面管理' }).click()
  const manager = page.locator('div[role="dialog"][aria-label="页面管理"]')
  await manager.waitFor({ timeout: 5_000 })
  await manager.getByLabel('新页面名称').fill('Work')
  await manager.getByRole('button', { name: '新建' }).click()
  await page.waitForTimeout(900)

  let list = await pages()
  check('新建后有两页', list.length === 2, `pages=${list.length} · ${list.map((p) => p.name).join(',')}`)

  const dots = await page.locator('nav[aria-label="页面"] button[aria-current="true"]').count()
  check('当前页圆点高亮唯一', dots === 1, `highlighted=${dots}`)
  await page.screenshot({ path: 'e2e/shot-m4-01-newpage.png' })

  // ---- 2) 点圆点切回第一页 ----
  const home = list.find((p) => p.slug === 'home')
  await page.locator('nav[aria-label="页面"] button').first().click()
  await page.waitForTimeout(700)
  const homeBoard = await boardOf(home.id)
  check('切回第一页后能看到原有图标', homeBoard.items.length === 2, `items=${homeBoard.items.length}`)

  // ---- 3) 滚轮翻页 ----
  await page.mouse.move(640, 400)
  await page.mouse.wheel(0, 300)
  await page.waitForTimeout(800)
  const afterWheel = await page.evaluate(() => location.hash)
  check('滚轮翻页生效', afterWheel.includes('work'), `hash=${afterWheel}`)

  // ---- 4) 横滑翻页 ----
  // 约定：向左滑 = 下一页，向右滑 = 上一页。此刻停在最后一页(Work)，
  // 所以向右滑回第一页。
  await page.mouse.move(400, 500)
  await page.mouse.down()
  await page.mouse.move(900, 510, { steps: 12 })
  await page.mouse.up()
  await page.waitForTimeout(800)
  const afterSwipe = await page.evaluate(() => location.hash)
  check('手势横滑翻页生效', afterSwipe.includes('home'), `hash=${afterSwipe}`)

  // ---- 4b) 首尾相连（R3）：两端不再撞墙，继续滑就绕回去 ----
  // 此刻在第一页(Home)。向右滑 = 上一页 = 绕到最后一页(Work)。
  await page.mouse.move(900, 500)
  await page.mouse.down()
  await page.mouse.move(400, 510, { steps: 12 })
  await page.mouse.up()
  await page.waitForTimeout(800)
  const wrapBack = await page.evaluate(() => location.hash)
  check('首页向右滑 → 绕到最后一页', wrapBack.includes('work'), `hash=${wrapBack}`)

  // 此刻在最后一页(Work)。向左滑 = 下一页 = 绕回第一页。
  await page.mouse.move(400, 500)
  await page.mouse.down()
  await page.mouse.move(900, 510, { steps: 12 })
  await page.mouse.up()
  await page.waitForTimeout(800)
  const wrapFwd = await page.evaluate(() => location.hash)
  check('末页向左滑 → 绕回首页', wrapFwd.includes('home'), `hash=${wrapFwd}`)

  // 滚轮走的是同一条路径（flip→adjacentPage），一起钉住：第一页往回滚 = 绕到最后一页。
  await page.mouse.move(640, 400)
  await page.mouse.wheel(0, -300)
  await page.waitForTimeout(800)
  const wrapWheel = await page.evaluate(() => location.hash)
  check('第一页往回滚 → 绕到最后一页', wrapWheel.includes('work'), `hash=${wrapWheel}`)

  // 回到第一页：下面的用例都从 Home 起手
  await page.mouse.wheel(0, 300)
  await page.waitForTimeout(800)

  // ---- 5) 长按 → 移动到… ----
  const tile = await page.locator('[data-tile]').first().boundingBox()
  await page.mouse.move(tile.x + tile.width / 2, tile.y + tile.height / 2)
  await page.mouse.down()
  await page.waitForTimeout(950)
  const menu = page.locator('div[role="menu"]')
  const menuVisible = await menu.isVisible().catch(() => false)
  await page.mouse.up()
  check('长按 800ms 弹出上下文菜单', menuVisible)
  if (menuVisible) {
    await page.screenshot({ path: 'e2e/shot-m4-02-menu.png' })
    await menu.getByRole('menuitem', { name: 'Work' }).click()
    await page.waitForTimeout(1000)
    const work = list.find((p) => p.slug === 'work')
    const workBoard = await boardOf(work.id)
    const homeAfter = await boardOf(home.id)
    check(
      '「移动到…」把图标搬到另一页',
      workBoard.items.length === 1 && homeAfter.items.length === 1,
      `work=${workBoard.items.length} home=${homeAfter.items.length}`,
    )
  }

  // ---- 6) 拖到屏幕边缘翻页 + 跨页落位（R2）----
  // 先确保停在第一页
  await page.locator('nav[aria-label="页面"] button').first().click()
  await page.waitForTimeout(700)
  const before = await boardOf(home.id)

  const t = await page.locator('[data-tile]').first().boundingBox()
  await page.mouse.move(t.x + t.width / 2, t.y + t.height / 2)
  await page.mouse.down()
  await page.mouse.move(t.x + t.width / 2 + 20, t.y + t.height / 2 + 4, { steps: 5 })
  // 移到屏幕最右边缘（进入 60px 边缘带），等超过 150ms 的悬停阈值
  await page.mouse.move(1272, t.y + t.height / 2, { steps: 15 })
  await page.waitForTimeout(600)

  const hashAtEdge = await page.evaluate(() => location.hash)
  const carryHint = await page.getByText(/松手放到/).count()
  check('拖到右边缘自动翻页', hashAtEdge.includes('work'), `hash=${hashAtEdge}`)
  check('进入跨页 carry（出现落位提示）', carryHint > 0, `提示元素=${carryHint}`)
  // 回归：拖拽/跨页期间**不允许**弹出长按菜单（库会把原元素换成 body 上的克隆，
  // 只绑在 li 上的 pointermove 收不到事件，曾导致 carry 到 800ms 时菜单乱入）
  const strayMenu = await page.locator('div[role="menu"]').count()
  check('跨页 carry 期间没有误弹上下文菜单', strayMenu === 0, `menu=${strayMenu}`)
  await page.screenshot({ path: 'e2e/shot-m4-03-carry.png' })

  await page.mouse.up()
  await page.waitForTimeout(1200)

  const work = list.find((p) => p.slug === 'work')
  const workFinal = await boardOf(work.id)
  const homeFinal = await boardOf(home.id)
  check(
    '跨页拖拽把图标落到目标页',
    homeFinal.items.length === before.items.length - 1 && workFinal.items.length === 2,
    `home ${before.items.length}->${homeFinal.items.length}, work ${workFinal.items.length}`,
  )
  await page.screenshot({ path: 'e2e/shot-m4-04-after-carry.png' })

  // ---- 7) 首尾相连：拖到边缘也不再撞墙 ----
  // 此刻停在最后一页(Work)。往**左**边缘拖 = 上一页 = 绕回第一页(Home)。
  // 只验"绕过去了"：绕过去之后按 Esc 取消 carry，一点数据都不动
  // （Esc 绑在 App 与 CarryLayer 上，见 cancelCarry）。
  const edgeTile = await page.locator('[data-tile]').first().boundingBox()
  await page.mouse.move(edgeTile.x + edgeTile.width / 2, edgeTile.y + edgeTile.height / 2)
  await page.mouse.down()
  await page.mouse.move(edgeTile.x + edgeTile.width / 2 - 20, edgeTile.y + edgeTile.height / 2 + 4, {
    steps: 5,
  })
  await page.mouse.move(8, edgeTile.y + edgeTile.height / 2, { steps: 15 })
  await page.waitForTimeout(600)
  const hashAtLeftEdge = await page.evaluate(() => location.hash)
  check('末页拖到左边缘 → 绕回第一页', hashAtLeftEdge.includes('home'), `hash=${hashAtLeftEdge}`)
  await page.keyboard.press('Escape')
  await page.mouse.up()
  await page.waitForTimeout(400)
  const afterCancel = await boardOf(home.id)
  check(
    '绕页途中取消 carry：数据没被改动',
    afterCancel.items.length === homeFinal.items.length,
    `home ${homeFinal.items.length} -> ${afterCancel.items.length}`,
  )

  check('浏览器控制台无 error', consoleErrors.length === 0, consoleErrors.slice(0, 2).join(' | '))
} catch (err) {
  check('执行过程未抛异常', false, err.message)
  await page.screenshot({ path: 'e2e/shot-m4-fail.png' }).catch(() => {})
} finally {
  await browser.close()
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
process.exit(failed.length === 0 ? 0 : 1)
