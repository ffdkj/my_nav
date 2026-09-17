/**
 * M2 真浏览器验收：启动真实后端 + headless Chromium，验证
 *   1) 初始渲染（网格 + 添加图块）
 *   2) 通过 UI 新增三个链接（走 LinkDialog → PUT board）
 *   3) 用鼠标真的拖拽一个图块，并确认顺序/坐标变化被持久化
 *   4) 刷新后顺序保持（证明落库而不是只在内存里）
 *   5) 搜索下拉出现站内模糊匹配
 * 任何一步失败即非零退出。
 *
 * 运行：node .e2e/smoke.mjs   （需要先 make web-build 与 go build）
 */
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18090'
const BOARD_URL = `${BASE}/api/pages/page-home/board`

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}

async function boardItems() {
  const res = await fetch(BOARD_URL)
  const json = await res.json()
  return json.items
    .slice()
    .sort((a, b) => a.row - b.row || a.col - b.col)
    .map((i) => ({ id: i.id, col: i.col, row: i.row, link: i.link_id }))
}

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })

const consoleErrors = []
page.on('console', (m) => {
  if (m.type() === 'error') consoleErrors.push(m.text())
})
page.on('pageerror', (e) => consoleErrors.push('pageerror: ' + e.message))

try {
  await page.goto(BASE, { waitUntil: 'networkidle' })

  // 1) 初始渲染
  const addTile = page.getByRole('button', { name: '添加图标' })
  await addTile.waitFor({ timeout: 10_000 })
  check('初始渲染：网格与「添加」图块存在', true)

  // 2) 通过 UI 添加三个链接
  const seeds = [
    { url: 'https://github.com', title: 'GitHub' },
    { url: 'https://gitee.com', title: 'Gitee' },
    { url: 'https://go.dev', title: 'Go' },
  ]
  for (const seed of seeds) {
    await addTile.click()
    await page.fill('#link-url-input', seed.url)
    await page.getByPlaceholder('例如 GitHub').fill(seed.title)
    await page.getByRole('button', { name: '确定' }).click()
    await page.getByRole('link', { name: new RegExp(seed.title) }).waitFor({ timeout: 10_000 })
  }
  const afterAdd = await boardItems()
  check('UI 新增三个链接并落库', afterAdd.length === 3, `items=${afterAdd.length}`)
  await page.screenshot({ path: '.e2e/shot-01-grid.png' })

  // 3) 鼠标拖拽：把第一个图块拖到第三个位置
  const tiles = page.locator('a[href="https://github.com"], a[href="https://gitee.com"], a[href="https://go.dev"]')
  const first = tiles.nth(0)
  const last = tiles.nth(2)
  const from = await first.boundingBox()
  const to = await last.boundingBox()
  if (!from || !to) throw new Error('拿不到图块坐标')

  await page.mouse.move(from.x + from.width / 2, from.y + from.height / 2)
  await page.mouse.down()
  // 先小幅移动越过拖拽阈值，再移动到大目标（分步是为了让库收到连续的 pointermove）
  await page.mouse.move(from.x + from.width / 2 + 12, from.y + from.height / 2 + 6, { steps: 5 })
  await page.mouse.move(to.x + to.width / 2, to.y + to.height / 2, { steps: 20 })
  await page.waitForTimeout(300)
  await page.mouse.up()
  await page.waitForTimeout(900)

  const afterDrag = await boardItems()
  check('拖拽后顺序发生变化', JSON.stringify(afterDrag) !== JSON.stringify(afterAdd))
  await page.screenshot({ path: '.e2e/shot-02-after-drag.png' })

  // 4) 刷新后仍是拖拽后的顺序（证明落库）
  await page.reload({ waitUntil: 'networkidle' })
  await page.locator('a[href="https://github.com"]').waitFor({ timeout: 10_000 })
  const afterReload = await boardItems()
  check(
    '刷新后顺序保持（已持久化）',
    JSON.stringify(afterReload) === JSON.stringify(afterDrag),
    `before=${JSON.stringify(afterDrag.map((i) => i.link))} after=${JSON.stringify(afterReload.map((i) => i.link))}`,
  )

  // 5) 搜索下拉：输入 git 应出现站内匹配
  await page.getByRole('textbox', { name: '搜索' }).fill('git')
  await page.waitForTimeout(400)
  const hitGithub = await page.locator('text=GitHub').count()
  check('站内模糊搜索出现结果', hitGithub > 0, `匹配元素数=${hitGithub}`)
  await page.screenshot({ path: '.e2e/shot-03-search.png' })

  // 6) 控制台无报错
  check('浏览器控制台无 error', consoleErrors.length === 0, consoleErrors.slice(0, 3).join(' | '))
} catch (err) {
  check('执行过程未抛异常', false, err.message)
  await page.screenshot({ path: '.e2e/shot-fail.png' }).catch(() => {})
} finally {
  await browser.close()
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
process.exit(failed.length === 0 ? 0 : 1)
