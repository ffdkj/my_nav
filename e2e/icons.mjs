/**
 * M5 真浏览器验收：图标抓取与编辑
 *
 * 关键点：测试里用一个**本地站点**作为目标（自带 <link rel="apple-touch-icon">），
 * 因此覆盖了完整链路 —— Google 端点会失败（本地域名它没有），
 * 于是走到"自建 HTML 发现"这一级，正是 M5 的核心逻辑。
 * 为此后端以 NAV_ALLOW_PRIVATE_FETCH=1 启动（默认关闭，见 README 的说明）。
 *
 *   1) 添加链接后自动抓到图标（<img src="/icons/..."> 出现）
 *   2) /icons/ 返回 immutable 缓存头
 *   3) 编辑面板：重新抓取仍是"已抓取"
 *   4) 切纯色文字（文字+颜色）→ 图块变成单色字，图片消失
 *   5) 上传本地 PNG → 图块变回 <img>，路径是上传的那个
 *   6) 重置为标准 favicon → 回到抓取结果
 */
import { createServer } from 'node:http'
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18090'

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}

// 1x1 红点 PNG（合法最小 PNG），用于"上传本地图标"这一步
const TINY_PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
)

// 一个"目标站点"：首页声明 apple-touch-icon，并真的提供该图标
const iconPng = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAACAAAAAgCAYAAABzenr0AAAAJUlEQVR42u3OMQEAAAgDoJnc6BpjDyQgd5cKBAKBQCAQCAQCgeDvAm0BAQGx4w0AAAAASUVORK5CYII=',
  'base64',
)
const site = createServer((req, res) => {
  if (req.url === '/touch.png') {
    res.writeHead(200, { 'Content-Type': 'image/png' })
    res.end(iconPng)
    return
  }
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
  res.end(`<!doctype html><html><head>
    <link rel="icon" sizes="16x16" href="/tiny.png">
    <link rel="apple-touch-icon" sizes="180x180" href="/touch.png">
  </head><body>local site</body></html>`)
})
await new Promise((resolve) => site.listen(0, '127.0.0.1', resolve))
const siteURL = `http://127.0.0.1:${site.address().port}/`


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

const tiles = () => page.locator('ul[aria-label="导航图标"] > li')
const tileIcon = () => tiles().first().locator('img')

try {
  await page.goto(BASE, { waitUntil: 'networkidle' })

  // ---- 1) 添加链接 → 自动抓图标 ----
  await page.getByRole('button', { name: '添加图标' }).click()
  await page.fill('#link-url-input', siteURL)
  await page.getByPlaceholder('例如 GitHub').fill('Local Site')
  await page.getByRole('button', { name: '确定' }).click()

  await tileIcon().waitFor({ timeout: 15_000 })
  const src = await tileIcon().getAttribute('src')
  check('添加后自动抓到图标', Boolean(src?.startsWith('/icons/')), `src=${src}`)

  const board = await (await fetch(`${BASE}/api/pages/page-home/board`)).json()
  const link = board.links[0]
  check('服务端记录了 icon_status=ok', link.icon_status === 'ok', `status=${link.icon_status}`)
  check('图标来自自建 HTML 发现', Boolean(link.icon_path), `path=${link.icon_path}`)

  // ---- 2) /icons/ 缓存头 ----
  const head = await fetch(`${BASE}${src}`, { method: 'HEAD' })
  const cache = head.headers.get('cache-control') ?? ''
  check('/icons/ 返回 immutable 缓存', head.status === 200 && cache.includes('immutable'), `cache=${cache}`)
  await page.screenshot({ path: 'e2e/shot-m5-01-fetched.png' })

  // ---- 3) 重新抓取 ----
  await tiles().first().locator('button[aria-label^="编辑"]').click({ force: true })
  const dialog = page.locator('form[aria-label="编辑图标"]')
  await dialog.waitFor({ timeout: 5_000 })
  await dialog.getByRole('button', { name: '重新抓取' }).click()
  await page.waitForTimeout(1200)
  const stillOk = await (await fetch(`${BASE}/api/pages/page-home/board`)).json()
  check('重新抓取后仍是已抓取', stillOk.links[0].icon_status === 'ok', `status=${stillOk.links[0].icon_status}`)

  // ---- 4) 纯色文字 ----
  await dialog.getByRole('tab', { name: '纯色文字' }).click()
  await dialog.locator('input[type="range"]').fill('48')
  await dialog.getByRole('button', { name: '颜色 #22C55E' }).click()
  await dialog.getByRole('button', { name: '应用纯色文字' }).click()
  await page.waitForTimeout(900)

  const mono = await (await fetch(`${BASE}/api/pages/page-home/board`)).json()
  check(
    '切换到纯色文字图标',
    mono.links[0].icon_source === 'monogram' && mono.links[0].icon_path === null,
    `source=${mono.links[0].icon_source} path=${mono.links[0].icon_path}`,
  )
  check('图块上的图片消失（改为单色字）', (await tileIcon().count()) === 0)
  await page.screenshot({ path: 'e2e/shot-m5-02-monogram.png' })

  // ---- 5) 上传本地图标 ----
  await dialog.getByRole('tab', { name: '本地图标' }).click()
  await dialog.locator('input[type="file"]').setInputFiles({
    name: 'mine.png',
    mimeType: 'image/png',
    buffer: TINY_PNG,
  })
  await page.waitForTimeout(1200)

  const uploaded = await (await fetch(`${BASE}/api/pages/page-home/board`)).json()
  check(
    '上传本地图标成功',
    uploaded.links[0].icon_source === 'upload' && uploaded.links[0].icon_status === 'ok',
    `source=${uploaded.links[0].icon_source} status=${uploaded.links[0].icon_status}`,
  )
  await tileIcon().waitFor({ timeout: 5_000 })
  check('图块重新显示图片', (await tileIcon().count()) === 1)
  await page.screenshot({ path: 'e2e/shot-m5-03-upload.png' })

  // ---- 6) 重置为标准 favicon ----
  await dialog.getByRole('tab', { name: '自动抓取' }).click()
  await dialog.getByRole('button', { name: '重置为标准 favicon' }).click()
  await page.waitForTimeout(1500)
  const reset = await (await fetch(`${BASE}/api/pages/page-home/board`)).json()
  check(
    '重置回标准 favicon',
    reset.links[0].icon_source === 'auto' && reset.links[0].icon_status === 'ok',
    `source=${reset.links[0].icon_source} status=${reset.links[0].icon_status}`,
  )

  check('浏览器控制台无 error', consoleErrors.length === 0, consoleErrors.slice(0, 2).join(' | '))
} catch (err) {
  check('执行过程未抛异常', false, err.message)
  await page.screenshot({ path: 'e2e/shot-m5-fail.png' }).catch(() => {})
} finally {
  await browser.close()
  site.close()
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
process.exit(failed.length === 0 ? 0 : 1)
