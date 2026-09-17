/**
 * 线上实例体检：对**已部署**的实例做一轮只读检查（不写入任何数据）。
 *
 *   NAV_BASE=http://100.70.0.29:8090 node e2e/deploy-check.mjs
 *
 * 检查项：服务健康、首屏、PWA 资源、非安全上下文下的降级（不注册 SW 且不报错）、
 * 已配置链接的图标抓取状态，并截图。升级后跑一遍即可确认没坏。
 */
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://100.70.0.29:8090'

const results = []
const check = (name, ok, detail = '') => {
  results.push({ name, ok })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}

// 1) HTTP 层
for (const [path, expectType] of [
  ['/healthz', 'application/json'],
  ['/', 'text/html'],
  ['/manifest.webmanifest', 'manifest+json'],
  ['/sw.js', 'javascript'],
]) {
  try {
    const res = await fetch(`${BASE}${path}`)
    const type = res.headers.get('content-type') ?? ''
    check(`GET ${path}`, res.ok && type.includes(expectType), `${res.status} ${type}`)
  } catch (err) {
    check(`GET ${path}`, false, err.message)
  }
}

// 2) 数据层与图标抓取
const boot = await (await fetch(`${BASE}/api/bootstrap`)).json()
check('bootstrap 可读', Array.isArray(boot.pages) && boot.engines.length > 0, `pages=${boot.pages.length} engines=${boot.engines.length}`)

const board = await (await fetch(`${BASE}/api/pages/${boot.pages[0].id}/board`)).json()
const ok = board.links.filter((l) => l.icon_status === 'ok').length
const miss = board.links.filter((l) => l.icon_status === 'miss').length
if (board.links.length > 0) {
  check('图标抓取正常（没有全部退化成兜底）', ok > 0 || miss === 0, `ok=${ok} miss=${miss}`)
}

// 3) 浏览器层：降级路径
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 720 } })
const errors = []
page.on('console', (m) => m.type() === 'error' && errors.push(m.text()))
page.on('pageerror', (e) => errors.push('pageerror: ' + e.message.split('\n')[0]))
await page.goto(BASE, { waitUntil: 'networkidle' })

const secure = await page.evaluate(() => window.isSecureContext)
const sw = await page.evaluate(async () => {
  if (!('serviceWorker' in navigator)) return 'unsupported'
  return (await navigator.serviceWorker.getRegistration()) ? 'registered' : 'none'
})
check('首屏渲染出网格', (await page.locator('h1').innerText()).includes('my_nav'))
check(
  secure ? 'PWA 已注册（HTTPS 环境）' : '非安全上下文下不注册 SW 且不报错',
  secure ? sw === 'registered' : sw !== 'registered' && errors.length === 0,
  `secure=${secure} sw=${sw} errors=${errors.length}`,
)
await page.screenshot({ path: 'e2e/shot-deployed.png' })
await browser.close()

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
process.exit(failed.length === 0 ? 0 : 1)
