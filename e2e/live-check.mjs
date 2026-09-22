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

  // ---- 触屏：`touch-action` 与"横滑能翻页"必须真的在线上生效 ----
  // 0.2.2 修的两条 bug（iPad 双开、手机滑不动）整套鼠标驱动的用例都看不见，
  // 所以体检里补一段**真触摸事件**的冒烟：只读（翻页只改浏览器 hash，不写数据）。
  const tctx = await browser.newContext({
    viewport: { width: 390, height: 844 },
    hasTouch: true,
    isMobile: true,
  })
  const tpage = await tctx.newPage()
  const cdp = await tctx.newCDPSession(tpage)
  await tpage.goto(BASE, { waitUntil: 'networkidle' })
  await tpage.waitForSelector('[data-testid="page-stage"]')
  await tpage.waitForTimeout(600)

  const ta = await tpage.evaluate(() => ({
    tile: getComputedStyle(document.querySelector('[data-tile]')).touchAction,
    main: getComputedStyle(document.querySelector('main')).touchAction,
  }))
  check(
    '触屏手势分工就位（图块 pan-y；横向留给 JS 翻页）',
    ta.tile === 'pan-y' && ta.main.startsWith('pan-y'),
    JSON.stringify(ta),
  )

  if ((await pages()).length >= 2) {
    const tileBox = await tpage.locator('[data-testid="tile-link"]').first().boundingBox()
    const from = { x: tileBox.x + tileBox.width / 2, y: tileBox.y + tileBox.height / 2 }
    const hashBefore = await tpage.evaluate(() => location.hash)
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{ x: from.x, y: from.y }],
    })
    for (let i = 1; i <= 10; i += 1) {
      await cdp.send('Input.dispatchTouchEvent', {
        type: 'touchMove',
        touchPoints: [{ x: Math.round(from.x - (200 * i) / 10), y: Math.round(from.y + (60 * i) / 10) }],
      })
    }
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
    await tpage.waitForTimeout(900)
    const hashAfter = await tpage.evaluate(() => location.hash)
    // 斜 16.7°（60/200）——旧的"纵向 <40px"绝对阈值会把它判成上下滑
    check(
      '从图块上起手、斜 16.7° 的真触摸横滑能翻页',
      hashAfter !== hashBefore,
      `${hashBefore} -> ${hashAfter}`,
    )

    // 首尾相连（决策 40）：跳到**最后一页**再向前滑一格，应该绕回**第一页**。
    // 不能写成"再滑一次回到出发页"——那只在两页时成立，线上现在有 4 页。
    // 起手点放在网格下方空白处：这一条只验"环"，"能在图块上起手"由上面那条负责。
    const slugs = (await pages()).map((p) => p.slug)
    try {
      const dots = tpage.locator('nav[aria-label="页面"] button')
      await dots.nth(slugs.length - 1).click()
      await tpage.waitForTimeout(900)
      const lastHash = await tpage.evaluate(() => location.hash)
      const y = Math.round((await tpage.evaluate(() => window.innerHeight)) * 0.75)
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: 320, y }] })
      for (let i = 1; i <= 10; i += 1) {
        await cdp.send('Input.dispatchTouchEvent', {
          type: 'touchMove',
          touchPoints: [{ x: Math.round(320 - 20 * i), y }],
        })
      }
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] })
      await tpage.waitForTimeout(900)
      const wrapHash = await tpage.evaluate(() => location.hash)
      check(
        '末页继续向前滑 → 首尾相连回到第一页',
        wrapHash.includes(slugs[0]) && !lastHash.includes(slugs[0]),
        `${lastHash} -> ${wrapHash}（首页 ${slugs[0]}，共 ${slugs.length} 页）`,
      )
      // 体检不该改变用户看到的页面：还原到第一页
      await dots.first().click()
      await tpage.waitForTimeout(700)
    } catch (err) {
      check('末页继续向前滑 → 首尾相连回到第一页', false, err.message)
    }
  } else {
    check('只有一个页面，跳过触屏横滑检查', true)
  }

  // 决策 40/41 的部署校验：只能证明"线上跑的确实是新包"
  const bundle = await tpage.evaluate(async () => {
    const src = document.querySelector('script[type="module"][src]')?.getAttribute('src') ?? ''
    const res = src ? await fetch(src) : null
    return { src, text: res ? await res.text() : '' }
  })
  check(
    '线上包里含新功能标记（搜索结果序号徽标）',
    bundle.text.includes('search-result-index'),
    bundle.src,
  )
  check(
    '到边提示已随首尾相连删除（死代码不在线上包里）',
    !bundle.text.includes('已经是最后一页') && !bundle.text.includes('已经是第一页'),
    bundle.src,
  )

  // 序号徽标要用**真实数据**验：查询词取自库里第一条链接的标题，保证一定有匹配
  const sample = await tpage.evaluate(async () => {
    const res = await fetch('/api/links')
    const json = await res.json()
    return ((json.links?.[0]?.title ?? '').slice(0, 3) || 'a').trim() || 'a'
  })
  const searchBox = tpage.getByRole('textbox', { name: '搜索' })
  await searchBox.fill(sample)
  await tpage.waitForTimeout(500)
  const firstBadge = await tpage
    .locator('[data-testid="search-result-index"]')
    .first()
    .textContent()
    .catch(() => null)
  check('搜索下拉结果带 1…N 序号徽标（真实数据）', firstBadge === '1', `查询=${sample} 徽标=${firstBadge}`)
  await searchBox.fill('')

  await tctx.close()
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
