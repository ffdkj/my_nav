/**
 * M3 真浏览器验收：文件夹全链路
 *   1) 拖到另一块上**悬停达阈值**后松手 → 合并成文件夹（并断言悬停期间出现"待合并"高亮）
 *   2) 小夹外显 9 个缩略位
 *   3) 点小夹 → 打开模态（role=dialog）
 *   4) 模态内"移出到主网格" → 夹内少一个、主网格多一个
 *   5) 编辑夹 → 切 2×2 → 网格上该块横跨 2 列（占 4 格）
 *   6) 大夹内部的图标是真实 <a href>，可直接点击
 */
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18090'
const BOARD_URL = `${BASE}/api/pages/page-home/board`

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}

async function board() {
  return (await fetch(BOARD_URL)).json()
}
const folderItems = (b) => b.items.filter((i) => i.kind === 'folder')
const linkItems = (b) => b.items.filter((i) => i.kind === 'link')


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

try {
  // 本脚本验证合并，明确要求阈值 500ms（并顺带验证了设置项真的生效）
  await fetch(`${BASE}/api/settings`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ merge_dwell_ms: '500' }),
  }).catch(() => {})
  const dwell = await (await fetch(`${BASE}/api/settings`)).json()
  check('合并阈值设置为 500ms', dwell.merge_dwell_ms === '500', `merge_dwell_ms=${dwell.merge_dwell_ms}`)

  await page.goto(BASE, { waitUntil: 'networkidle' })

  // 准备两个链接
  const addTile = page.getByRole('button', { name: '添加图标' })
  for (const s of [
    { url: 'https://github.com', title: 'GitHub' },
    { url: 'https://gitee.com', title: 'Gitee' },
  ]) {
    await addTile.click()
    await page.fill('#link-url-input', s.url)
    await page.getByPlaceholder('例如 GitHub').fill(s.title)
    await page.getByRole('button', { name: '确定' }).click()
    await page.getByRole('link', { name: new RegExp(s.title) }).waitFor({ timeout: 10_000 })
  }

  // ---- 1) 悬停 dwell 合并 ----
  const a = await page.locator('a[href="https://github.com"]').boundingBox()
  const b = await page.locator('a[href="https://gitee.com"]').boundingBox()
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2)
  await page.mouse.down()
  await page.mouse.move(a.x + a.width / 2 + 14, a.y + a.height / 2 + 6, { steps: 5 })
  await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 20 })
  // 阈值默认 500ms；停住 800ms 让它武装
  await page.waitForTimeout(800)
  const armedCount = await page.locator('.scale-105').count()
  check('悬停达阈值出现「待合并」高亮', armedCount > 0, `高亮元素=${armedCount}`)
  await page.screenshot({ path: 'e2e/shot-m3-01-armed.png' })

  await page.mouse.up()
  await page.waitForTimeout(900)

  await waitSaved(page)
  let bd = await board()
  const folders = folderItems(bd)
  check('合并后生成 1 个文件夹', folders.length === 1, `folders=${folders.length}`)
  check(
    '文件夹内含 2 个图标',
    folders[0]?.children?.length === 2,
    `children=${folders[0]?.children?.length}`,
  )
  check('页面上不再有裸链接块', linkItems(bd).length === 0, `links=${linkItems(bd).length}`)
  await page.screenshot({ path: 'e2e/shot-m3-02-folder.png' })

  // ---- 2) 小夹外显 9 个缩略位 ----
  const slots = await page.locator('button[aria-label^="打开"] .grid > *').count()
  check('小夹外显 9 个缩略位', slots === 9, `slots=${slots}`)

  // ---- 3) 点击打开模态 ----
  await page.locator('button[aria-label^="打开"]').click()
  const modal = page.locator('div[role="dialog"]')
  await modal.waitFor({ timeout: 5_000 })
  check('点击小夹打开模态', await modal.isVisible())
  await page.screenshot({ path: 'e2e/shot-m3-03-modal.png' })

  // ---- 4) 移出到主网格 ----
  await page.locator('button[aria-label="移出到主网格"]').first().click()
  await page.waitForTimeout(700)
  await waitSaved(page)
  bd = await board()
  check(
    '移出后夹内剩 1 个、主网格多 1 个',
    folderItems(bd)[0]?.children?.length === 1 && linkItems(bd).length === 1,
    `children=${folderItems(bd)[0]?.children?.length} links=${linkItems(bd).length}`,
  )
  await page.keyboard.press('Escape')
  await page.locator('button[aria-label="关闭"]').click().catch(() => {})
  await page.waitForTimeout(300)

  // ---- 5) 切成 2x2 大夹 ----
  await page.locator('button[aria-label^="编辑"]').first().click()
  const fdialog = page.locator('div[role="dialog"][aria-label="编辑文件夹"]')
  await fdialog.waitFor({ timeout: 5_000 })
  await fdialog.getByRole('button', { name: /2×2/ }).click()
  await fdialog.getByRole('button', { name: '确定' }).click()
  await page.waitForTimeout(800)

  bd = await board()
  const size = folderItems(bd)[0]?.size
  check('文件夹尺寸变为 2', size === 2, `size=${size}`)

  const span = await page
    .locator('ul[aria-label="导航图标"] > li')
    .first()
    .evaluate((el) => getComputedStyle(el).gridColumn)
  check('大夹在网格横跨 2 列（占 4 格）', /span 2/.test(span), `grid-column=${span}`)
  await page.screenshot({ path: 'e2e/shot-m3-04-big.png' })

  // ---- 6) 大夹内部图标可直接点击 ----
  const innerLinks = await page.locator('ul[aria-label="导航图标"] > li').first().locator('a[href]').count()
  check('大夹内部图标是真实链接', innerLinks >= 1, `a[href]=${innerLinks}`)

  check('浏览器控制台无 error', consoleErrors.length === 0, consoleErrors.slice(0, 2).join(' | '))
} catch (err) {
  check('执行过程未抛异常', false, err.message)
  await page.screenshot({ path: 'e2e/shot-m3-fail.png' }).catch(() => {})
} finally {
  await browser.close()
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
process.exit(failed.length === 0 ? 0 : 1)
