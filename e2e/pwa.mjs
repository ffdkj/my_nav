/**
 * M8 真浏览器验收：PWA
 *
 * 关键前提：Service Worker 只在**安全上下文**注册。
 * 127.0.0.1 属于安全上下文，所以本地能测到完整 PWA 行为；
 * 而真实部署是 http://<tailnet-ip>:8090（非安全上下文），SW 会注册失败。
 * 因此这里**两条路径都要验**：
 *   A) 安全上下文：manifest 正确、SW 装上并接管、离线仍能打开
 *   B) 非安全上下文：不报错、不注册、应用照常可用（PWA 只是增强，不是依赖）
 */
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18090'
const LAN_BASE = process.env.NAV_LAN_BASE ?? ''

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}
function skip(name, why) {
  console.log(`SKIP  ${name}  -> ${why}`)
}

const browser = await chromium.launch()

// ---------- A) 安全上下文（127.0.0.1） ----------
{
  const context = await browser.newContext()
  const page = await context.newPage()
  const errors = []
  page.on('console', (m) => {
    if (m.type() === 'error') errors.push(m.text())
  })
  page.on('pageerror', (e) => errors.push('pageerror: ' + e.message.split('\n')[0]))

  try {
    await page.goto(BASE, { waitUntil: 'networkidle' })

    const manifestHref = await page.getAttribute('link[rel="manifest"]', 'href')
    check('注入了 manifest 链接', manifestHref === '/manifest.webmanifest', `href=${manifestHref}`)

    const manifest = await (await fetch(`${BASE}/manifest.webmanifest`)).json()
    check(
      'manifest 字段完整',
      manifest.name === 'my_nav' && manifest.display === 'standalone' && manifest.start_url === '/',
      `display=${manifest.display} start_url=${manifest.start_url}`,
    )
    const iconSizes = (manifest.icons ?? []).map((i) => i.sizes)
    check(
      'manifest 含 192/512 与 maskable 图标',
      iconSizes.includes('192x192') &&
        iconSizes.includes('512x512') &&
        (manifest.icons ?? []).some((i) => i.purpose === 'maskable'),
      `sizes=${iconSizes.join(',')}`,
    )

    const manifestRes = await fetch(`${BASE}/manifest.webmanifest`)
    check(
      'manifest 的 Content-Type 正确',
      (manifestRes.headers.get('content-type') ?? '').includes('manifest+json'),
      `type=${manifestRes.headers.get('content-type')}`,
    )

    const swRes = await fetch(`${BASE}/sw.js`)
    const swCache = swRes.headers.get('cache-control') ?? ''
    check('sw.js 不被长缓存', !swCache.includes('immutable'), `cache=${swCache}`)

    // 等待 Service Worker 安装
    await page.waitForFunction(
      async () => {
        const reg = await navigator.serviceWorker.getRegistration()
        return Boolean(reg?.active)
      },
      null,
      { timeout: 20_000 },
    )
    check('Service Worker 已安装并激活', true)

    // 首次加载时 SW 尚未接管当前页面，重载一次即可被接管
    await page.reload({ waitUntil: 'networkidle' })
    const controlled = await page.evaluate(() => Boolean(navigator.serviceWorker.controller))
    check('页面已被 Service Worker 接管', controlled)
    await page.screenshot({ path: 'e2e/shot-m8-01-online.png' })

    // 离线仍能打开（应用外壳来自 precache，数据来自 NetworkFirst 的缓存）
    await page.getByRole('button', { name: '添加图标' }).click()
    await page.fill('#link-url-input', 'https://example.com')
    await page.getByPlaceholder('例如 GitHub').fill('Cached')
    await page.getByRole('button', { name: '确定' }).click()
    await page.waitForFunction(() => !document.body.innerText.includes('保存中'), null, {
      timeout: 20_000,
    })
    await page.waitForTimeout(1500) // 让 NetworkFirst 把 /api 响应写进缓存

    await context.setOffline(true)
    await page.reload({ waitUntil: 'domcontentloaded' })
    await page.waitForTimeout(2000)

    const offlineTitle = await page.locator('h1').innerText().catch(() => '')
    const offlineTiles = await page.locator('ul[aria-label="导航图标"] > li').count()
    check('离线仍能打开应用外壳', offlineTitle.includes('my_nav'), `title=${offlineTitle}`)
    check('离线时导航数据来自缓存', offlineTiles >= 1, `tiles=${offlineTiles}`)
    await page.screenshot({ path: 'e2e/shot-m8-02-offline.png' })

    await context.setOffline(false)
    check('安全上下文下控制台无 error', errors.length === 0, errors.slice(0, 2).join(' | '))
  } catch (err) {
    check('安全上下文流程未抛异常', false, err.message)
    await page.screenshot({ path: 'e2e/shot-m8-fail.png' }).catch(() => {})
  }
  await context.close()
}

// ---------- B) 非安全上下文（局域网 IP，模拟真实部署） ----------
if (!LAN_BASE) {
  skip('非安全上下文降级', '未提供 NAV_LAN_BASE')
} else {
  const context = await browser.newContext()
  const page = await context.newPage()
  const errors = []
  page.on('console', (m) => {
    if (m.type() === 'error') errors.push(m.text())
  })
  page.on('pageerror', (e) => errors.push('pageerror: ' + e.message.split('\n')[0]))

  try {
    await page.goto(LAN_BASE, { waitUntil: 'networkidle' })
    const secure = await page.evaluate(() => window.isSecureContext)
    check('局域网访问确实是非安全上下文', secure === false, `isSecureContext=${secure}`)

    const title = await page.locator('h1').innerText()
    const tiles = await page.locator('ul[aria-label="导航图标"] > li').count()
    check('非安全上下文下应用照常可用', title.includes('my_nav') && tiles >= 1, `tiles=${tiles}`)

    const reg = await page.evaluate(async () => {
      if (!('serviceWorker' in navigator)) return 'unsupported'
      const r = await navigator.serviceWorker.getRegistration()
      return r ? 'registered' : 'none'
    })
    check('非安全上下文下不注册 Service Worker', reg !== 'registered', `registration=${reg}`)
    await page.screenshot({ path: 'e2e/shot-m8-03-insecure.png' })

    check('降级路径控制台无 error', errors.length === 0, errors.slice(0, 2).join(' | '))
  } catch (err) {
    check('非安全上下文流程未抛异常', false, err.message)
  }
  await context.close()
}

await browser.close()
const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
process.exit(failed.length === 0 ? 0 : 1)
