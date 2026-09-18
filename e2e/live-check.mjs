/**
 * 线上实例的**只读**体检（部署后跑一遍，确认"装上去的确实是新版本、而且真的对"）。
 *
 *   NAV_BASE=http://100.70.0.29:8090 node e2e/live-check.mjs
 *
 * ⚠️ 只读约定：只用 GET + 客户端行为观察（翻页只改浏览器 hash）。
 * 任何人可能正在用这个实例，所以：
 *   - 断言**不变量**而不是固定值（例如"页面主题必须等于服务端此刻的设置"），
 *     否则真人随手点一下就会让检查变红；
 *   - 不 PATCH 设置、不新增/删除数据、不触发图标抓取。
 *
 * 与 e2e/*.mjs 的区别：那些用例会起自己的临时实例并写数据，这个只"看"。
 */
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE
if (!BASE) {
  console.error('需要 NAV_BASE，例如 NAV_BASE=http://100.70.0.29:8090 node e2e/live-check.mjs')
  process.exit(2)
}

const results = []
const check = (name, ok, detail = '') => {
  results.push({ name, ok })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}

const settings = async () => await (await fetch(`${BASE}/api/settings`)).json()
const pages = async () => (await (await fetch(`${BASE}/api/pages/`)).json()).pages
const bootstrap = async () => await (await fetch(`${BASE}/api/bootstrap`)).json()

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })
const errors = []
page.on('console', (m) => m.type() === 'error' && errors.push(m.text()))
page.on('pageerror', (e) => errors.push('pageerror: ' + e.message.split('\n')[0]))

/** 页面里逐帧记录换页动画的 inline style（computed transform 在 headless 下不可信） */
const sampler = (ms) =>
  new Promise((resolve) => {
    const frames = []
    const t0 = performance.now()
    const tick = () => {
      const stage = document.querySelector('[data-testid="page-stage"]')
      const ghost = document.querySelector('[data-testid="page-ghost"]')
      const track = document.querySelector('[data-testid="wallpaper-track"]')
      frames.push({
        stage: (stage?.getAttribute('style') || '').replace(/transition:[^;]*;?/g, '').trim(),
        ghost: (ghost?.getAttribute('style') || '').replace(/transition:[^;]*;?/g, '').trim(),
        track: track ? (track.getAttribute('style') || '').replace(/transition:[^;]*;?/g, '').trim() : null,
      })
      if (performance.now() - t0 < ms) requestAnimationFrame(tick)
      else resolve(frames)
    }
    requestAnimationFrame(tick)
  })

try {
  const boot = await bootstrap()
  const st = await settings()
  check('服务端可访问', Boolean(boot?.pages?.length), `pages=${boot?.pages?.length}`)

  await page.goto(BASE, { waitUntil: 'networkidle' })
  await page.waitForSelector('[data-testid="page-stage"]')
  await page.waitForTimeout(600)

  const dom = await page.evaluate(() => {
    const root = document.documentElement
    const tile = document.querySelector('[data-testid="tile-link"]')
    // 图块里可能是真图标，也可能是纯色兜底（例如这个站没抓到图标）——
    // 两种都该"撑满"，所以取哪个存在就量哪个
    const art = document.querySelector('[data-testid="tile-icon"], [data-testid="tile-monogram"]')
    // 撑满规则有**两个**合法档，逐块量（只量第一块会在"第一块恰好是小位图"时误报）：
    // 常规图撑满 100%；源图 <64px 的小位图只放大到 60%（硬拉会糊，spec 决策 33）
    const fills = [...document.querySelectorAll('[data-testid="tile-link"]')]
      .map((t) => {
        const a = t.querySelector('[data-testid="tile-icon"], [data-testid="tile-monogram"]')
        if (!a) return null
        return {
          kind: a.getAttribute('data-testid') === 'tile-icon' ? 'icon' : 'word',
          w: a.getBoundingClientRect().width / t.getBoundingClientRect().width,
          nat: a.naturalWidth || 0,
        }
      })
      .filter(Boolean)
    const label = document.querySelector('[data-testid="tile-label"]')
    const bar = document.querySelector('input[aria-label="搜索"]')?.closest('div')
    const radius = getComputedStyle(root).getPropertyValue('--radius-tile').trim()
    return {
      dark: root.classList.contains('dark'),
      dataTheme: root.dataset.theme,
      photo: root.dataset.photo ?? '',
      scrim: Boolean(document.querySelector('[data-testid="wallpaper-scrim"]')),
      wallpaper: document.querySelector('[data-testid="wallpaper"]')?.getAttribute('src') ?? null,
      fill: art && tile ? art.getBoundingClientRect().width / tile.getBoundingClientRect().width : null,
      artKind: art ? art.getAttribute('data-testid') : null,
      fills,
      labelOpacity: label ? getComputedStyle(label).opacity : null,
      barBg: bar ? getComputedStyle(bar).backgroundColor : null,
      radius,
      slideMs: getComputedStyle(root).getPropertyValue('--page-slide-ms').trim(),
      tiles: document.querySelectorAll('[data-testid="tile-link"]').length,
    }
  })

  // 主题：只看"与此刻的服务端设置是否一致"，绝不断言某个固定值
  const serverDark = st.theme === 'dark' || (st.theme === 'auto' && dom.dark)
  check(
    '页面主题与服务端设置一致（theme=' + st.theme + '）',
    dom.dark === serverDark && dom.dataTheme === (dom.dark ? 'dark' : 'light'),
    JSON.stringify({ dark: dom.dark, dataTheme: dom.dataTheme }),
  )

  // 蒙版与玻璃：深色 + 有照片 才该有蒙版
  const wantScrim = dom.dark && dom.photo === '1'
  check(
    '蒙版规则正确（深色+照片才铺黑蒙版；浅色一律不铺）',
    dom.scrim === wantScrim,
    JSON.stringify({ dark: dom.dark, photo: dom.photo, scrim: dom.scrim }),
  )
  if (dom.photo === '1' && !dom.dark) {
    check('浅色 + 照片：玻璃翻成白色半透明', dom.barBg === 'rgba(255, 255, 255, 0.55)', dom.barBg)
  } else {
    check('无照片 / 深色：玻璃用默认那一档', Boolean(dom.barBg), dom.barBg)
  }

  check('图块形状变量就位（默认圆角方形 1rem）', dom.radius !== '', `--radius-tile=${dom.radius}`)
  check('动画时长是 300ms', ['.3s', '0.3s', '300ms'].includes(dom.slideMs), dom.slideMs)

  if (dom.tiles > 0) {
    const off = dom.fills.filter((f) => !(f.w > 0.95 || (f.w > 0.55 && f.w < 0.65)))
    check(
      '图标撑满图块（满格，或设计里的 60% 小位图档；没有中间态）',
      dom.fills.length > 0 && off.length === 0,
      `fills=${dom.fills.map((f) => f.w.toFixed(2)).join(',')} 越界=${off.length}`,
    )
    check('标题默认不显示（悬停才浮出）', dom.labelOpacity === '0', `opacity=${dom.labelOpacity}`)
    await page.locator('[data-testid="tile-link"]').first().hover()
    await page.waitForTimeout(250)
    const hovered = await page.evaluate(
      () => getComputedStyle(document.querySelector('[data-testid="tile-label"]')).opacity,
    )
    check('悬停后标题浮出', Number(hovered) > 0.9, `opacity=${hovered}`)
    await page.mouse.move(0, 0)
  } else {
    check('当前页没有图块，跳过撑满/悬停检查', true)
  }

  // 大夹：空白处开预览模态 + 夹内每个图标都有 ✎（0.2.1 的新行为）
  // 只读：不点 ✎（那会去查候选、往图标库里写字节），点完模态也马上关掉。
  const bigOpen = page.locator('[data-testid="bigfolder-open"]')
  if ((await bigOpen.count()) > 0) {
    // ⚠️ 层级必须在**开模态之前**探：模态是 fixed inset-0，
    // 开了之后 elementFromPoint 只会打到那层遮罩。
    const layering = await page.evaluate(() => {
      const li = document.querySelector('[data-testid="bigfolder-open"]')?.closest('li')
      const a = li?.querySelector('a[href]')
      if (!a) return 'no-icon'
      const r = a.getBoundingClientRect()
      const hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)
      return hit?.closest('a[href]') ? 'link' : 'covered'
    })
    check('大夹里的图标本身仍可直接点（没被热区盖住）', layering === 'link', `hit=${layering}`)

    // 按钮中心被九宫格盖着，Playwright 会判定被拦截 —— 点左上角内边距
    await bigOpen.first().click({ position: { x: 4, y: 4 } })
    const modal = page.locator('[data-testid="folder-modal"]')
    const opened = await modal
      .waitFor({ timeout: 5_000 })
      .then(() => true)
      .catch(() => false)
    check('点大夹空白处打开与小夹同一个预览模态', opened)

    if (opened) {
      const pencils = await modal.locator('[data-testid="folder-link-edit"]').count()
      const kids = await modal.locator('li a[href]').count()
      check(
        '预览模态内每个夹内图标都有 ✎（改图标入口）',
        kids > 0 && pencils === kids,
        `✎=${pencils} 夹内图标=${kids}`,
      )
      await page.screenshot({ path: 'e2e/shot-live-folder-modal.png' }).catch(() => {})
      await modal.locator('button[aria-label="关闭"]').click()
      await page.waitForTimeout(300)
    }
  } else {
    check('线上当前页没有 2×2 大夹，跳过预览模态检查', true)
  }

  // 换页方向：两个方向必须相反（这是 0.2.0 修掉的那个 bug 的线上回归）
  const list = await pages()
  if (list.length >= 2) {
    const flip = async (from, to) => {
      await page.locator('nav[aria-label="页面"] button').nth(from).click()
      await page.waitForTimeout(700)
      const p = page.evaluate(sampler, 700)
      await page.locator('nav[aria-label="页面"] button').nth(to).click()
      const frames = await p
      await page.waitForTimeout(400)
      const withStage = frames.filter((f) => f.stage.includes('translate3d'))
      const pct = (s) => {
        const m = /translate3d\((-?[\d.]+)%/.exec(s || '')
        return m ? Number(m[1]) : null
      }
      const tracks = frames.filter((f) => f.track)
      return {
        stageStart: pct(withStage[0]?.stage),
        ghostStart: pct(withStage[0]?.ghost),
        trackFirst: pct(tracks[0]?.track),
        trackLast: pct(tracks[tracks.length - 1]?.track),
        trackSeen: tracks.length > 0,
      }
    }
    const next = await flip(0, 1)
    const prev = await flip(1, 0)
    check('下一个页：图标从右侧进（起点 +100%）', next.stageStart === 100, JSON.stringify(next))
    check('上一个页：图标从左侧进（起点 -100%）', prev.stageStart === -100, JSON.stringify(prev))
    if (next.trackSeen || prev.trackSeen) {
      check(
        '壁纸轨道方向跟着图标（下一页 0→-50，上一页 -50→0）',
        next.trackFirst === 0 && next.trackLast === -50 && prev.trackFirst === -50 && prev.trackLast === 0,
        JSON.stringify({ next, prev }),
      )
    } else {
      check('两页壁纸相同 → 壁纸层不平移（符合预期）', true)
    }
  } else {
    check('只有一个页面，跳过换页方向检查', true)
  }

  check('浏览器控制台无 error', errors.length === 0, errors.slice(0, 2).join(' | '))
} catch (err) {
  check('检查过程未抛异常', false, err.message)
} finally {
  await browser.close()
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
if (failed.length) {
  console.log('失败项：')
  for (const f of failed) console.log('  - ' + f.name)
}
process.exit(failed.length ? 1 : 0)
