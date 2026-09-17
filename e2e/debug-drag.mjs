/** 拖拽诊断：逐步打印 DOM 顺序、元素 transform、以及是否发出 PUT。 */
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18090'
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })

const puts = []
page.on('request', (r) => {
  if (r.method() === 'PUT' && r.url().includes('/board')) {
    try {
      puts.push(JSON.parse(r.postData() ?? '{}').items?.map((i) => i.id.slice(-4)) ?? [])
    } catch {
      puts.push('unparsable')
    }
  }
})
page.on('pageerror', (e) => console.log('PAGEERROR:', e.message.split('\n')[0]))
page.on('console', (m) => { const t = m.text(); if (!t.includes('DevTools')) console.log('CONSOLE[' + m.type() + ']:', t.slice(0, 200)) })

await page.goto(BASE, { waitUntil: 'networkidle' })

const addTile = page.getByRole('button', { name: '添加图标' })
for (const s of [
  { url: 'https://github.com', title: 'GitHub' },
  { url: 'https://gitee.com', title: 'Gitee' },
  { url: 'https://go.dev', title: 'Go' },
]) {
  await addTile.click()
  await page.fill('#link-url-input', s.url)
  await page.getByPlaceholder('例如 GitHub').fill(s.title)
  await page.getByRole('button', { name: '确定' }).click()
  await page.getByRole('link', { name: new RegExp(s.title) }).waitFor()
}

const domOrder = () =>
  page.$$eval('ul[aria-label="导航图标"] a', (els) => els.map((e) => e.getAttribute('href')))
const transforms = () =>
  page.$$eval('ul[aria-label="导航图标"] li', (els) => els.map((e) => e.style.transform || '-'))

console.log('列数 =', await page.getAttribute('div[data-cols]', 'data-cols'))
console.log('初始 DOM 顺序 =', await domOrder())

const first = await page.locator('ul[aria-label="导航图标"] a').nth(0).boundingBox()
const last = await page.locator('ul[aria-label="导航图标"] a').nth(2).boundingBox()
const cx = (b) => b.x + b.width / 2
const cy = (b) => b.y + b.height / 2

console.log('起点中心 =', cx(first).toFixed(0), cy(first).toFixed(0), ' 终点中心 =', cx(last).toFixed(0), cy(last).toFixed(0))

await page.mouse.move(cx(first), cy(first))
await page.waitForTimeout(80)
await page.mouse.down()
console.log('mousedown 后 DOM =', await domOrder(), 'transform =', await transforms())

for (const dx of [3, 6, 12, 24, 48]) {
  await page.mouse.move(cx(first) + dx, cy(first) + 2, { steps: 3 })
  await page.waitForTimeout(60)
  console.log(`移动 +${dx}px 后 transform =`, await transforms())
}

await page.mouse.move(cx(last), cy(last), { steps: 20 })
await page.waitForTimeout(250)
console.log('到达目标后 DOM =', await domOrder(), 'transform =', await transforms())

await page.mouse.up()
await page.waitForTimeout(800)
console.log('mouseup 后 DOM =', await domOrder())
console.log('PUT 次数 =', puts.length, JSON.stringify(puts))

await browser.close()
