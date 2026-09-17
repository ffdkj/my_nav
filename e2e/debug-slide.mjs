/**
 * 换页平移动画诊断：逐帧打印舞台（page-stage）的 rect / inline transform，
 * 以及壁纸轨道是否存在。动画看起来"没生效"时先跑它。
 *
 * 用法：NAV_BASE=http://127.0.0.1:18200 node e2e/debug-slide.mjs
 * 需要一个已经起好的后端；页面不足 2 页时脚本会自己建一页。
 *
 * 为什么需要它：transform 过渡跑在合成器线程，headless 下
 * `getComputedStyle().transform` 与 `getBoundingClientRect()` 可能整段停在
 * 起点或终点（两个方向表现还不一样）。肉眼判断"到底动没动"要看这里逐帧的数据，
 * 写断言则只能用 inline style / transition 属性这类主线程必然可见的状态。
 */
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18200'
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })
page.on('pageerror', (e) => console.log('[pageerror]', e.message))
await page.goto(BASE, { waitUntil: 'networkidle' })

let pages = (await (await fetch(`${BASE}/api/pages/`)).json()).pages
if (pages.length < 2) {
  await page.evaluate(async () => {
    await fetch('/api/pages/', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ id: 'dbg-page-2', slug: 'dbg-2', name: 'Debug2' }),
    })
  })
  await page.reload({ waitUntil: 'networkidle' })
  await page.waitForTimeout(400)
  pages = (await (await fetch(`${BASE}/api/pages/`)).json()).pages
}
console.log(`页面：${pages.map((p) => p.name).join(', ')}`)

async function watch(label, dotIndex) {
  await page.evaluate(() => {
    const stage = document.querySelector('[data-testid="page-stage"]')
    window.__log = []
    window.__stop = false
    const t0 = performance.now()
    const tick = () => {
      const r = stage.getBoundingClientRect()
      // Web Animations API 能看到真正在跑的 CSSTransition（不受合成器线程读数影响）
      const anims = (stage.getAnimations?.() ?? []).map((a) => ({
        prop: a.transitionProperty ?? a.constructor.name,
        t: a.currentTime,
        state: a.playState,
      }))
      const track = document.querySelector('[data-testid="wallpaper-track"]')
      const trackAnims = (track?.getAnimations?.() ?? []).map((a) => ({
        prop: a.transitionProperty ?? a.constructor.name,
        t: a.currentTime,
      }))
      window.__log.push({
        t: Math.round(performance.now() - t0),
        w: Math.round(r.width),
        left: Math.round(r.left),
        computed: getComputedStyle(stage).transform,
        inline: stage.style.transform,
        anims,
        track: Boolean(track),
        trackAnims,
      })
      if (!window.__stop && performance.now() - t0 < 900) requestAnimationFrame(tick)
    }
    requestAnimationFrame(tick)
  })
  await page.locator('nav[aria-label="页面"] button').nth(dotIndex).click()
  await page.waitForTimeout(1000)
  const log = await page.evaluate(() => {
    window.__stop = true
    return window.__log
  })
  console.log(`--- ${label} ---`)
  for (const r of log) {
    const a = r.anims.map((x) => `${x.prop}@${Math.round(x.t)}ms/${x.state}`).join(',')
    const ta = r.trackAnims.map((x) => `${x.prop}@${Math.round(x.t)}ms`).join(',')
    console.log(
      `  +${String(r.t).padStart(4)}ms left=${String(r.left).padStart(6)} track=${r.track ? 'Y' : 'n'}` +
        ` stageAnim=[${a}] trackAnim=[${ta}] inline="${r.inline}" computed=${r.computed}`,
    )
  }
}

await watch('下一页 0 → 1', 1)
await page.waitForTimeout(700)
await watch('上一页 1 → 0', 0)
await browser.close()
