/**
 * 临时探针 v3（跑完即删）：在**线上实例**上实测换页平移的真实方向。
 *
 * v1/v2 的教训：
 *  - 动画只有 260ms，且结束后 stage 的 transform 会被清空 —— 只对首末帧取 rect 得到的是假象；
 *  - v2 的采样循环被 stop 之后没有重启，方向 B 一帧都没采到。
 * 所以这里：每个方向**重装**一次采样器（在页内自己按时间停），只读 DOM 上的
 * `style` **属性字符串**（computed transform 在 headless 里会被合成器冻住，不可信）。
 */
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE
if (!BASE) {
  console.error('需要 NAV_BASE')
  process.exit(2)
}

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })
page.on('pageerror', (e) => console.log('pageerror:', e.message.split('\n')[0]))
await page.goto(BASE, { waitUntil: 'load' })
await page.waitForSelector('[data-testid="page-stage"]')
await page.waitForTimeout(600)

const sampler = (ms) =>
  new Promise((resolve) => {
    const el = (s) => document.querySelector(s)
    const attrs = (s) => {
      const e = el(s)
      if (!e) return null
      return {
        style: (e.getAttribute('style') || '').replace(/\s+/g, ' ').trim(),
        ghostKids: s.includes('ghost') ? e.childElementCount : undefined,
      }
    }
    const frames = []
    const t0 = performance.now()
    const tick = () => {
      frames.push({
        t: Math.round(performance.now() - t0),
        stage: attrs('[data-testid="page-stage"]'),
        ghost: attrs('[data-testid="page-ghost"]'),
        track: attrs('[data-testid="wallpaper-track"]'),
        stageH: el('[data-testid="page-stage"]')?.firstElementChild?.childElementCount ?? -1,
      })
      if (performance.now() - t0 < ms) requestAnimationFrame(tick)
      else resolve(frames)
    }
    requestAnimationFrame(tick)
  })

async function dir(label, wheel) {
  const framesP = page.evaluate(sampler, 700)
  await page.mouse.move(640, 400)
  await page.mouse.wheel(0, wheel)
  await page.waitForTimeout(90)
  const anims = await page.evaluate(() => {
    const out = {}
    for (const sel of ['page-stage', 'page-ghost', 'wallpaper-track']) {
      const e = document.querySelector(`[data-testid="${sel}"]`)
      out[sel] = e ? e.getAnimations().map((a) => `${a.transitionProperty}@${a.effect?.getTiming?.().duration} ${a.playState}`) : null
    }
    return out
  })
  const frames = await framesP
  await page.waitForTimeout(500)

  // 只看动画窗口：stage 的 style 属性里出现 transform 的那些帧
  const win = frames.filter((f) => (f.stage?.style || '').includes('transform'))
  const gwin = frames.filter((f) => (f.ghost?.style || '').includes('translateX'))
  const tw = frames.filter((f) => (f.track?.style || '').includes('transform'))
  const fmt = (f, k) => (f?.[k]?.style || '').replace(/transition:[^;]*;?/g, '').trim()

  console.log(`\n===== ${label} =====`)
  console.log('hash:', await page.evaluate(() => location.hash))
  console.log('getAnimations:', JSON.stringify(anims))
  console.log('图标动画帧数:', win.length, '| ghost 动画帧数:', gwin.length)
  console.log('  stage 第1帧:', fmt(win[0], 'stage'))
  console.log('  stage 末  帧:', fmt(win[win.length - 1], 'stage'))
  console.log('  ghost 第1帧:', fmt(gwin[0], 'ghost'), '| ghost 有内容:', gwin[0]?.ghost?.ghostKids)
  console.log('  ghost 末  帧:', fmt(gwin[gwin.length - 1], 'ghost'))
  console.log('壁纸轨道帧数:', tw.length)
  if (tw.length) {
    console.log('  track 第1帧:', fmt(tw[0], 'track'))
    console.log('  track 末  帧:', fmt(tw[tw.length - 1], 'track'))
  }
  const deltas = []
  const activeFrames = frames.filter((f) => f.t >= (win[0]?.t ?? 1e9) - 20 && f.t <= (win[win.length - 1]?.t ?? -1) + 20)
  for (let i = 1; i < activeFrames.length; i++) deltas.push(activeFrames[i].t - activeFrames[i - 1].t)
  const avg = deltas.length ? deltas.reduce((a, b) => a + b, 0) / deltas.length : 0
  console.log('  帧间隔: avg', avg.toFixed(1), 'ms | worst', deltas.length ? Math.max(...deltas) : 0, 'ms | 掉帧(>20ms)', deltas.filter((d) => d > 20).length, '/', deltas.length)
  return {
    stageFrom: fmt(win[0], 'stage'),
    ghostFrom: fmt(gwin[0], 'ghost'),
    trackFrom: fmt(tw[0], 'track'),
    trackTo: fmt(tw[tw.length - 1], 'track'),
    kids: tw[0] ? undefined : undefined,
  }
}

const a = await dir('方向 A：滚轮向下 → 下一页（Home → Job）', 200)
const b = await dir('方向 B：滚轮向上 → 上一页（Job → Home）', -200)

console.log('\n===== 结论 =====')
const num = (s) => {
  const m = /translateX\((-?[\d.]+)%\)/.exec(s || '')
  return m ? Number(m[1]) : null
}
console.log('A 图标入场起点:', num(a.stageFrom), '% | 出场起点:', num(a.ghostFrom), '%')
console.log('B 图标入场起点:', num(b.stageFrom), '% | 出场起点:', num(b.ghostFrom), '%')
console.log('A 壁纸轨道:', a.trackFrom, '→', a.trackTo)
console.log('B 壁纸轨道:', b.trackFrom, '→', b.trackTo)
const opposite = (x, y) => x !== null && y !== null && x * y < 0
console.log(opposite(num(a.stageFrom), num(b.stageFrom)) ? '图标层：两方向相反 ✓' : '图标层：两方向相同 ✗')

await browser.close()
