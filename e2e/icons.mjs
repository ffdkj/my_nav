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
// 这两张会在浏览器起来之后用 canvas 生成真实尺寸的图（默认给 32×32 占位）
let touchPng = iconPng
let bigPng = iconPng
const site = createServer((req, res) => {
  if (req.url === '/touch.png') {
    res.writeHead(200, { 'Content-Type': 'image/png' })
    res.end(touchPng)
    return
  }
  if (req.url === '/big.png') {
    res.writeHead(200, { 'Content-Type': 'image/png' })
    res.end(bigPng)
    return
  }
  if (req.url === '/app.webmanifest') {
    res.writeHead(200, { 'Content-Type': 'application/manifest+json' })
    res.end(JSON.stringify({ icons: [{ src: '/big.png', sizes: '512x512', type: 'image/png' }] }))
    return
  }
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
  res.end(`<!doctype html><html><head>
    <link rel="icon" sizes="16x16" href="/tiny.png">
    <link rel="apple-touch-icon" sizes="180x180" href="/touch.png">
    <link rel="manifest" href="/app.webmanifest">
  </head><body>local site</body></html>`)
})

/** 在页面里用 canvas 造一张 PNG（返回 base64），供上面的站点夹具使用 */
const makePngInPage = ([w, h, from, to]) => {
  const canvas = document.createElement('canvas')
  canvas.width = w
  canvas.height = h
  const ctx = canvas.getContext('2d')
  const grad = ctx.createLinearGradient(0, 0, w, h)
  grad.addColorStop(0, from)
  grad.addColorStop(1, to)
  ctx.fillStyle = grad
  ctx.fillRect(0, 0, w, h)
  return canvas.toDataURL('image/png').split(',')[1]
}
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
const dialog = page.locator('form[aria-label="编辑图标"]')

const openEdit = async () => {
  await tiles().first().locator('button[aria-label^="编辑"]').click({ force: true })
  await dialog.waitFor({ timeout: 5_000 })
}

try {
  await page.goto(BASE, { waitUntil: 'networkidle' })

  // 站点夹具换成真实的 180×180 / 512×512（尺寸断言才有意义）
  touchPng = Buffer.from(await page.evaluate(makePngInPage, [180, 180, '#2563eb', '#22d3ee']), 'base64')
  bigPng = Buffer.from(await page.evaluate(makePngInPage, [512, 512, '#0f172a', '#64748b']), 'base64')

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

  // ---- 2.5) 图标撑满图块 + 文字悬停浮出 ----
  const fill = await page.evaluate(() => {
    const img = document.querySelector('[data-testid="tile-icon"]')
    const tile = img?.closest('[data-testid="tile-link"]')
    const label = document.querySelector('[data-testid="tile-label"]')
    if (!img || !tile || !label) return null
    const ir = img.getBoundingClientRect()
    const tr = tile.getBoundingClientRect()
    return {
      ratioW: ir.width / tr.width,
      ratioH: ir.height / tr.height,
      labelOpacity: getComputedStyle(label).opacity,
      overflow: getComputedStyle(tile).overflow,
    }
  })
  check(
    '图标撑满整个图块（宽高都 ≥95%）',
    Boolean(fill) && fill.ratioW > 0.95 && fill.ratioH > 0.95,
    JSON.stringify(fill),
  )
  check('文字默认不显示（悬停才浮出）', fill?.labelOpacity === '0', `opacity=${fill?.labelOpacity}`)
  await tiles().first().locator('[data-testid="tile-link"]').hover()
  await page.waitForTimeout(300)
  const hovered = await page.evaluate(
    () => getComputedStyle(document.querySelector('[data-testid="tile-label"]')).opacity,
  )
  check('悬停后文字压条浮出', Number(hovered) > 0.9, `opacity=${hovered}`)
  await page.mouse.move(0, 0)
  await page.waitForTimeout(200)

  // ---- 3) 重新抓取 ----
  await openEdit()
  await dialog.getByRole('button', { name: '重新抓取' }).click()
  await page.waitForTimeout(1500)
  const stillOk = await (await fetch(`${BASE}/api/pages/page-home/board`)).json()
  check('重新抓取后仍是已抓取', stillOk.links[0].icon_status === 'ok', `status=${stillOk.links[0].icon_status}`)
  await dialog.getByRole('button', { name: '取消' }).click()
  await page.waitForTimeout(300)

  // ---- 3.5) 候选：输入网址后自动列出多张，选中并确定 → 手选生效 ----
  await openEdit()
  const cards = dialog.locator('[data-testid="icon-card-candidate"]')
  await cards.first().waitFor({ timeout: 20_000 })
  const cardCount = await cards.count()
  check('编辑面板自动列出候选图标', cardCount >= 2, `count=${cardCount}`)

  // 候选卡片的 title 里带着出处/尺寸，按 180×180 找 apple-touch 那张
  const touchCard = cards.filter({ hasText: '180×180' }).first()
  const touchTitle = await touchCard.getAttribute('title')
  check(
    '候选带出处与尺寸信息（apple-touch-icon 180×180）',
    Boolean(touchTitle?.includes('apple-touch-icon')),
    touchTitle,
  )
  await touchCard.click()
  await page.waitForTimeout(200)
  await page.screenshot({ path: 'e2e/shot-m11-01-candidate-cards.png' })
  await dialog.getByRole('button', { name: '确定' }).click()
  await page.waitForTimeout(1200)

  const picked = (await (await fetch(`${BASE}/api/pages/page-home/board`)).json()).links[0]
  check(
    '手选生效：记下了来源地址 icon_picked_url',
    Boolean(picked.icon_picked_url?.endsWith('/touch.png')),
    `picked=${picked.icon_picked_url}`,
  )
  check(
    '手选的尺寸也入库（180×180，服务端自己 Inspect 的结果）',
    picked.icon_w === 180 && picked.icon_h === 180 && picked.icon_status === 'ok',
    `w=${picked.icon_w} h=${picked.icon_h} status=${picked.icon_status}`,
  )
  const pickedSrc = await tileIcon().getAttribute('src')
  check('图块换成手选的那张', pickedSrc === `/icons/${picked.icon_path}`, `src=${pickedSrc}`)
  await page.screenshot({ path: 'e2e/shot-m5-04-candidates.png' })

  // ---- 4) 纯色文字 ----
  await openEdit()
  await dialog.getByTestId('icon-card-monogram').click()
  await dialog.locator('input[type="range"]').fill('48')
  await dialog.getByRole('button', { name: '颜色 #22C55E' }).click()
  await dialog.getByRole('button', { name: '确定' }).click()
  await page.waitForTimeout(1200)

  const mono = (await (await fetch(`${BASE}/api/pages/page-home/board`)).json()).links[0]
  check(
    '切换到纯色文字图标',
    mono.icon_source === 'monogram' && mono.icon_path === null,
    `source=${mono.icon_source} path=${mono.icon_path}`,
  )
  check('图块上的图片消失（改为单色字）', (await tileIcon().count()) === 0)
  check('纯色字图标铺满整块', (await page.locator('[data-testid="tile-monogram"]').count()) === 1)
  await page.screenshot({ path: 'e2e/shot-m5-02-monogram.png' })

  // 纯色是"用户明确选的"：改个网址不该把它抹掉（spec §6 的守卫）
  await openEdit()
  await page.fill('#link-url-input', `${siteURL}?v=2`)
  await dialog.getByRole('button', { name: '确定' }).click()
  await page.waitForTimeout(1000)
  const afterURL = (await (await fetch(`${BASE}/api/pages/page-home/board`)).json()).links[0]
  check(
    '改网址不会把用户选的纯色图标重置掉',
    afterURL.icon_source === 'monogram',
    `source=${afterURL.icon_source} url=${afterURL.url}`,
  )

  // ---- 5) 上传本地图标 ----
  await openEdit()
  await dialog.getByTestId('icon-card-upload').click()
  await dialog.locator('input[type="file"]').setInputFiles({
    name: 'mine.png',
    mimeType: 'image/png',
    buffer: TINY_PNG,
  })
  await page.waitForTimeout(300)
  await dialog.getByRole('button', { name: '确定' }).click()
  await page.waitForTimeout(1200)

  const uploaded = (await (await fetch(`${BASE}/api/pages/page-home/board`)).json()).links[0]
  check(
    '上传本地图标成功',
    uploaded.icon_source === 'upload' && uploaded.icon_status === 'ok',
    `source=${uploaded.icon_source} status=${uploaded.icon_status}`,
  )
  await tileIcon().waitFor({ timeout: 5_000 })
  check('图块重新显示图片', (await tileIcon().count()) === 1)
  await page.screenshot({ path: 'e2e/shot-m5-03-upload.png' })

  // ---- 6) 重置为标准 favicon ----
  await openEdit()
  await dialog.getByRole('button', { name: '重置为标准 favicon' }).click()
  await page.waitForTimeout(2000)
  const reset = (await (await fetch(`${BASE}/api/pages/page-home/board`)).json()).links[0]
  check(
    '重置回标准 favicon',
    reset.icon_source === 'auto' && reset.icon_status === 'ok',
    `source=${reset.icon_source} status=${reset.icon_status}`,
  )
  check(
    '重置也清掉了手选标记（icon_picked_url 为空）',
    reset.icon_picked_url === null,
    `picked=${reset.icon_picked_url}`,
  )
  await dialog.getByRole('button', { name: '取消' }).click()
  await page.waitForTimeout(300)

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
