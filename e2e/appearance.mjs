/**
 * 真浏览器验收：主题（白天/黑夜）· 每页壁纸 · 换页平移动画
 *
 * 这三件事的共同点是"看起来已经做完了，其实没接上"，所以断言全部走用户视角
 * （DOM 类名 / 计算样式 / 服务端数据），不看组件内部状态：
 *
 *  1) 主题选择器早就存在，但代码里**一个 dark: 变体都没有**、body 底色写死深色，
 *     壁纸蒙版也是写死的黑渐变 —— 于是点按钮毫无反应，浅色主题下满屏发暗。
 *  2) "设为本页"把 wallpaper_mode / wallpaper_id 写进了**全局 settings**，
 *     页面自己的值只留在内存里 —— 刷新后被服务端打回 global，
 *     用户看到的就是"换了壁纸全局都变，而且设置又改回跟随全局"。
 *  3) 换页没有任何过渡。现在要求：图标平移、壁纸只在**真的换了**才平移、
 *     搜索栏/设置那些固定 UI 一动不动。
 */
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18090'

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}

const settings = async () => (await fetch(`${BASE}/api/settings`)).json()
const wallpapers = async () => (await (await fetch(`${BASE}/api/wallpapers`)).json()).wallpapers
const pages = async () => (await (await fetch(`${BASE}/api/pages/`)).json()).pages

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })
const consoleErrors = []
page.on('console', (m) => {
  if (m.type() === 'error') consoleErrors.push(m.text())
})
page.on('pageerror', (e) => consoleErrors.push('pageerror: ' + e.message.split('\n')[0]))

async function makePng(width, height, from, to) {
  return page.evaluate(
    ([w, h, a, b]) => {
      const canvas = document.createElement('canvas')
      canvas.width = w
      canvas.height = h
      const ctx = canvas.getContext('2d')
      const grad = ctx.createLinearGradient(0, 0, w, h)
      grad.addColorStop(0, a)
      grad.addColorStop(1, b)
      ctx.fillStyle = grad
      ctx.fillRect(0, 0, w, h)
      return canvas.toDataURL('image/png').split(',')[1]
    },
    [width, height, from, to],
  )
}

/**
 * 页面内的采样器：换页动画的"平移状态机"证据。
 *
 * ⚠️ 位移量只能当参考，不能当断言：transform 过渡跑在合成器线程上，
 * headless 下 getBoundingClientRect / getComputedStyle().transform 有时反映、
 * 有时**整段停在起点或终点**（实测同样的动画，两个方向表现还不一样）——
 * 拿它做断言就是随机失败。
 * 所以断言全部落在主线程**必然**看得到的东西上：inline style 的内容
 * （起点 ±100% → 终点 0%）、transition 属性真的挂在元素上、快照与轨道在不在、
 * 以及固定 UI 的 rect（它们没有 transform，读数稳定）。
 */
async function armSampler() {
  await page.evaluate(() => {
    const stage = document.querySelector('[data-testid="page-stage"]')
    const ghost = document.querySelector('[data-testid="page-ghost"]')
    const fixed = [
      document.querySelector('[data-testid="theme-toggle"]'),
      document.querySelector('header h1'),
    ]
    const trace = {
      frames: 0,
      styledFrames: 0,
      startPct: 0,
      sawEnd: false,
      transitionProp: '',
      transitionDur: '',
      animFrames: 0,
      animProp: '',
      animMaxTime: 0,
      ghostVisible: false,
      ghostItems: 0,
      track: false,
      trackFirst: '',
      trackInline: '',
      slidingFrames: 0,
      maxShift: 0,
      fixedMoved: false,
    }
    window.__trace = trace
    const stageLeft = stage.getBoundingClientRect().left
    const base = fixed.map((el) => el.getBoundingClientRect().left)
    const t0 = performance.now()
    const tick = () => {
      trace.frames++
      // inline style 是"动画状态机"的直接证据（transition 存在期间一直挂着）
      const inline = stage.style.transform
      if (inline) {
        trace.styledFrames++
        // 平移用 translate3d（强制合成层），所以正则也要跟着换
        const pct = /translate3d\((-?[\d.]+)%/.exec(inline)
        if (pct && !trace.startPct) trace.startPct = parseFloat(pct[1])
        if (/translate3d\(0%/.test(inline)) trace.sawEnd = true
        if (document.documentElement.dataset.pageSliding === '1') trace.slidingFrames++
        if (!trace.transitionDur) {
          const cs = getComputedStyle(stage)
          trace.transitionProp = cs.transitionProperty
          trace.transitionDur = cs.transitionDuration
        }
      }
      // Web Animations API：真正在跑的 CSSTransition（不受合成器读数影响，见文件头注释）
      for (const a of stage.getAnimations?.() ?? []) {
        if (a.transitionProperty) {
          trace.animFrames++
          trace.animProp = a.transitionProperty
          trace.animMaxTime = Math.max(trace.animMaxTime, Number(a.currentTime) || 0)
        }
      }
      trace.maxShift = Math.max(
        trace.maxShift,
        Math.abs(stage.getBoundingClientRect().left - stageLeft),
      )
      if (ghost && ghost.childElementCount > 0) {
        trace.ghostItems = Math.max(trace.ghostItems, ghost.childElementCount)
        if (getComputedStyle(ghost).display !== 'none') trace.ghostVisible = true
      }
      const track = document.querySelector('[data-testid="wallpaper-track"]')
      if (track) {
        trace.track = true
        // 首帧也要记：壁纸层"往哪边平移"完全取决于起点/终点这一对值
        if (!trace.trackFirst) trace.trackFirst = track.style.transform
        trace.trackInline = track.style.transform
      }
      fixed.forEach((el, i) => {
        if (Math.abs(el.getBoundingClientRect().left - base[i]) > 0.5) trace.fixedMoved = true
      })
      if (performance.now() - t0 < 1500) requestAnimationFrame(tick)
    }
    requestAnimationFrame(tick)
  })
}

const readTrace = () => page.evaluate(() => window.__trace)

/** 点第 n 个圆点翻页，并取回这段时间的采样 */
async function flipTo(dotIndex) {
  await armSampler()
  await page.waitForTimeout(80) // 让采样先转起来
  await page.locator('nav[aria-label="页面"] button').nth(dotIndex).click()
  await page.waitForTimeout(900) // 等动画跑完 + 收尾
  return readTrace()
}

const htmlState = () =>
  page.evaluate(() => {
    const root = document.documentElement
    const css = getComputedStyle(root)
    return {
      dark: root.classList.contains('dark'),
      theme: root.dataset.theme,
      mode: root.dataset.themeMode,
      fg: css.getPropertyValue('--c-fg').trim(),
      bodyBg: getComputedStyle(document.body).backgroundColor,
      colorScheme: css.colorScheme,
    }
  })

const wallpaperSrc = () =>
  page.locator('img[data-testid="wallpaper"]').getAttribute('src')

try {
  await page.goto(BASE, { waitUntil: 'networkidle' })

  // ---------- 1) 默认深色 ----------
  let st = await htmlState()
  check('初始为深色（种子 theme=dark）', st.dark && st.fg === '#e5edff', JSON.stringify(st))
  check('深色下 body 底色是深蓝黑', st.bodyBg === 'rgb(7, 11, 20)', st.bodyBg)
  // 此时还没有任何壁纸：兜底底色应当干净，不该再压一层暗色蒙版（用户报的"整页蒙了一层黑"）
  let bare = await page.evaluate(() => ({
    scrim: Boolean(document.querySelector('[data-testid="wallpaper-scrim"]')),
    fallbackBg: getComputedStyle(
      document.querySelector('[data-testid="wallpaper-fallback"]'),
    ).backgroundColor,
  }))
  check(
    '没有壁纸时不铺蒙版（深色）',
    bare.scrim === false && bare.fallbackBg === 'rgb(7, 11, 20)',
    JSON.stringify(bare),
  )

  // ---------- 2) 一键切到白天 ----------
  await page.locator('[data-testid="theme-toggle"]').click()
  await page.waitForTimeout(400)
  st = await htmlState()
  check(
    '点按钮切到白天：.dark 移除、色板翻转、color-scheme 跟着变',
    !st.dark && st.theme === 'light' && st.fg === '#0b1220' && st.colorScheme === 'light',
    JSON.stringify(st),
  )
  check('白天 body 底色变浅', st.bodyBg === 'rgb(238, 241, 248)', st.bodyBg)
  bare = await page.evaluate(() => ({
    scrim: Boolean(document.querySelector('[data-testid="wallpaper-scrim"]')),
    fallbackBg: getComputedStyle(
      document.querySelector('[data-testid="wallpaper-fallback"]'),
    ).backgroundColor,
  }))
  check(
    '没有壁纸时不铺蒙版，且兜底底色跟着主题变亮（白天）',
    bare.scrim === false && bare.fallbackBg === 'rgb(238, 241, 248)',
    JSON.stringify(bare),
  )
  check('主题写入服务端设置', (await settings()).theme === 'light', (await settings()).theme)
  await page.screenshot({ path: 'e2e/shot-m10-01-light.png' })

  // 组件层的 token 也必须跟着翻（只翻 body 不算修好）
  await page.getByRole('button', { name: '打开设置' }).click()
  const dialog = page.locator('div[role="dialog"][aria-label="设置"]')
  await dialog.waitFor({ timeout: 5_000 })
  const dialogBg = await dialog.evaluate((el) => getComputedStyle(el).backgroundColor)
  check('白天下面板底色是白的（组件 token 也跟着翻了）', dialogBg === 'rgb(255, 255, 255)', dialogBg)

  // ---------- 3) 面板里切回深色 + 刷新后仍是深色 ----------
  await dialog.getByTestId('theme-select').selectOption('dark')
  await page.waitForTimeout(500)
  st = await htmlState()
  check('面板里切回深色', st.dark && st.mode === 'dark', JSON.stringify(st))
  await page.reload({ waitUntil: 'networkidle' })
  await page.waitForTimeout(400)
  st = await htmlState()
  check('刷新后主题保持深色', st.dark && (await settings()).theme === 'dark', JSON.stringify(st))

  // ---------- 4) 两张壁纸 ----------
  await page.getByRole('button', { name: '打开设置' }).click()
  await dialog.waitFor({ timeout: 5_000 })
  await dialog.getByRole('tab', { name: '壁纸' }).click()
  const pngBig = await makePng(800, 600, '#123456', '#fedcba')
  await dialog.locator('input[aria-label="上传壁纸"]').setInputFiles({
    name: 'wide.png',
    mimeType: 'image/png',
    buffer: Buffer.from(pngBig, 'base64'),
  })
  await page.waitForTimeout(1800)
  const pngSmall = await makePng(400, 300, '#22c55e', '#0ea5e9')
  await dialog.locator('input[aria-label="上传壁纸"]').setInputFiles({
    name: 'small.png',
    mimeType: 'image/png',
    buffer: Buffer.from(pngSmall, 'base64'),
  })
  await page.waitForTimeout(1800)

  const list = await wallpapers()
  const W1 = list.find((w) => w.w === 800)
  const W2 = list.find((w) => w.w === 400)
  check('上传了两张壁纸', Boolean(W1 && W2), `count=${list.length}`)

  const items = dialog.locator('[data-testid="wallpaper-item"]')
  /** 列表里按缩略图文件名找那一项（顺序=上传顺序，但按文件名找更稳） */
  const itemOf = async (w) => {
    const n = await items.count()
    for (let i = 0; i < n; i++) {
      const src = await items.nth(i).locator('img').getAttribute('src')
      if (src?.includes(w.thumb_file ?? w.file ?? '###')) return { item: items.nth(i) }
    }
    throw new Error(`列表里找不到壁纸 ${w.id}`)
  }

  // ---------- 5) W1 = 全局；W2 = 本页（Home） ----------
  await (await itemOf(W1)).item.getByRole('button', { name: '设为全局' }).click()
  await page.waitForTimeout(700)
  check('全局壁纸设置成功', (await settings()).wallpaper_id === W1.id)

  await (await itemOf(W2)).item.getByRole('button', { name: '设为本页' }).click()
  await page.waitForTimeout(700)

  const homeId = (await pages()).find((p) => p.name === 'Home').id
  const globalAfter = (await settings()).wallpaper_id
  const home = (await pages()).find((p) => p.id === homeId)
  check(
    '「设为本页」不再改全局壁纸（本次核心回归）',
    globalAfter === W1.id,
    `global=${globalAfter} 期望=${W1.id}`,
  )
  check(
    '本页壁纸写进 pages 行而不是 settings',
    home.wallpaper_mode === 'custom' && home.wallpaper_id === W2.id,
    `mode=${home.wallpaper_mode} id=${home.wallpaper_id}`,
  )
  check('画面上生效的是本页那张', (await wallpaperSrc())?.includes(W2.file), await wallpaperSrc())
  check(
    '列表里标出了 全局 / 本页',
    (await (await itemOf(W1)).item.getAttribute('data-role')) === 'global' &&
      (await (await itemOf(W2)).item.getAttribute('data-role')) === 'page',
  )

  await dialog.getByRole('button', { name: '关闭设置' }).click()
  await page.waitForTimeout(400)

  // ---------- 6) 刷新后本页壁纸仍在（不会被"改回跟随全局"）----------
  await page.reload({ waitUntil: 'networkidle' })
  await page.waitForTimeout(800)
  const homeAfter = (await pages()).find((p) => p.id === homeId)
  check(
    '刷新后本页壁纸没有被改回跟随全局',
    homeAfter.wallpaper_mode === 'custom' && homeAfter.wallpaper_id === W2.id,
    `mode=${homeAfter.wallpaper_mode}`,
  )
  check('刷新后画面仍是本页那张', (await wallpaperSrc())?.includes(W2.file), await wallpaperSrc())

  // ---------- 7) 新建第二页：它跟随全局（W1）----------
  await page.getByRole('button', { name: '页面管理' }).click()
  const manager = page.locator('div[role="dialog"][aria-label="页面管理"]')
  await manager.waitFor({ timeout: 5_000 })
  await manager.getByLabel('新页面名称').fill('Work')
  await manager.getByRole('button', { name: '新建' }).click()
  await page.waitForTimeout(1500)
  const work = (await pages()).find((p) => p.name === 'Work')
  check('新建页默认跟随全局', work?.wallpaper_mode === 'global', `mode=${work?.wallpaper_mode}`)
  check('跟随全局时看到的是全局那张', (await wallpaperSrc())?.includes(W1.file), await wallpaperSrc())

  // ---------- 8) 换页动画：壁纸不同 → 壁纸也跟着平移 ----------
  // 当前在 Work（全局 W1），点第 1 个圆点回 Home（本页 W2）
  let tr = await flipTo(0)
  check(
    '换页时图标层被挂上平移：起点在画面外 → 落到 0，并且真的在跑 transform 过渡',
    tr.styledFrames > 0 &&
      tr.startPct === -100 &&
      tr.sawEnd &&
      tr.transitionProp.includes('transform') &&
      tr.transitionDur !== '' &&
      tr.transitionDur !== '0s' &&
      tr.animProp === 'transform' &&
      tr.animMaxTime > 80,
    `styled=${tr.styledFrames} start=${tr.startPct}% end=${tr.sawEnd} ${tr.transitionDur} 过渡=${tr.animProp}@${Math.round(tr.animMaxTime)}ms 实测位移=${tr.maxShift.toFixed(0)}px`,
  )
  check('换页时旧的网格快照出现（旧页滑出去）', tr.ghostVisible && tr.ghostItems > 0, JSON.stringify(tr))
  check(
    '壁纸不同 → 壁纸层也平移，方向与图标一致（上一页：轨道 -50% → 0，整体向右）',
    tr.track === true && tr.trackFirst.includes('-50%') && tr.trackInline.includes('0%'),
    `track=${tr.track} first="${tr.trackFirst}" last="${tr.trackInline}"`,
  )
  check(
    '动画期间 <html data-page-sliding> 挂上（用来临时关掉玻璃面的模糊）',
    tr.slidingFrames > 0,
    `slidingFrames=${tr.slidingFrames}`,
  )
  check('固定 UI（标题与主题按钮）不参与平移', tr.fixedMoved === false)
  const settled = await page.evaluate(() => {
    const stage = document.querySelector('[data-testid="page-stage"]')
    const ghost = document.querySelector('[data-testid="page-ghost"]')
    return {
      inline: stage.style.transform,
      ghostDisplay: getComputedStyle(ghost).display,
      ghostChildren: ghost.childElementCount,
      track: Boolean(document.querySelector('[data-testid="wallpaper-track"]')),
    }
  })
  check(
    '动画结束后回到静止态（无残留 transform / 快照 / 轨道）',
    settled.inline === '' &&
      settled.ghostDisplay === 'none' &&
      settled.ghostChildren === 0 &&
      !settled.track,
    JSON.stringify(settled),
  )
  check('回到 Home 后仍是本页那张壁纸', (await wallpaperSrc())?.includes(W2.file), await wallpaperSrc())

  // 反向再翻一页：Home → Work（下一页方向），此刻两页壁纸仍然不同（W2 vs 全局 W1），
  // 正好用来验证壁纸层"另一个方向"是真的反向 —— 这正是之前两个方向都往左的回归点。
  await page.waitForTimeout(200)
  tr = await flipTo(1)
  check(
    '下一页且壁纸不同 → 轨道 0% → -50%（与上一页方向相反）',
    tr.track === true && tr.trackFirst.includes('0%') && tr.trackInline.includes('-50%'),
    `track=${tr.track} first="${tr.trackFirst}" last="${tr.trackInline}"`,
  )
  check('图标层方向与之相反（起点 +100%）', tr.startPct === 100, `startPct=${tr.startPct}`)
  await page.locator('nav[aria-label="页面"] button').nth(1).click()
  await page.waitForTimeout(90)
  await page.screenshot({ path: 'e2e/shot-m10-02-slide.png' })
  await page.waitForTimeout(700)

  // ---------- 9) 壁纸相同 → 壁纸层不动，只有图标平移到上一页（方向相反）----------
  // Home 已经是自定义 W2；现在切到 Work（dialog 必须关着，遮罩会挡住圆点）
  await page.locator('nav[aria-label="页面"] button').nth(1).click()
  await page.waitForTimeout(700)
  await page.getByRole('button', { name: '打开设置' }).click()
  await dialog.waitFor({ timeout: 5_000 })
  await dialog.getByRole('tab', { name: '壁纸' }).click()
  await (await itemOf(W2)).item.getByRole('button', { name: '设为本页' }).click()
  await page.waitForTimeout(700)
  check(
    '两页都指向 W2（壁纸一致）',
    (await pages()).every((p) => p.wallpaper_id === W2.id && p.wallpaper_mode === 'custom'),
  )
  await dialog.getByRole('button', { name: '关闭设置' }).click()
  await page.waitForTimeout(400)

  // 先回 Home（不做断言），再从 Home → Work（下一页，方向相反）采样
  await page.locator('nav[aria-label="页面"] button').first().click()
  await page.waitForTimeout(800)
  tr = await flipTo(1)
  check(
    '下一页：起点 +100%（方向与上一页相反），同样真的在跑过渡',
    tr.styledFrames > 0 && tr.startPct === 100 && tr.sawEnd && tr.animMaxTime > 80,
    `styled=${tr.styledFrames} startPct=${tr.startPct} 过渡=${tr.animProp}@${Math.round(tr.animMaxTime)}ms 实测位移=${tr.maxShift.toFixed(0)}px`,
  )
  check('壁纸一致 → 壁纸层完全不平移', tr.track === false && tr.ghostVisible)

  // ---------- 10) 蒙版与玻璃：白天不压白蒙版，改由白色玻璃保证可读 ----------
  // 此时在 Work，壁纸是 W2（两页都指向它）。
  // 用头部的主题按钮切换（设置面板的 tab 会停在上次打开的「壁纸」上，用 select 得先切 tab）
  await page.locator('[data-testid="theme-toggle"]').click()
  await page.waitForTimeout(500)
  check('主题按钮切到白天并写入服务端', (await settings()).theme === 'light', (await settings()).theme)

  const lightLook = await page.evaluate(() => {
    const tile = document.querySelector('[data-testid="tile-link"], [data-testid="folder-tile"]')
    const bar = document.querySelector('input[aria-label="搜索"]')?.closest('div')
    const dot = document.querySelector('nav[aria-label="页面"] button')
    const root = getComputedStyle(document.documentElement)
    return {
      photo: document.documentElement.dataset.photo,
      scrim: Boolean(document.querySelector('[data-testid="wallpaper-scrim"]')),
      tileBg: tile ? getComputedStyle(tile).backgroundColor : null,
      barBg: bar ? getComputedStyle(bar).backgroundColor : null,
      dotBg: dot ? getComputedStyle(dot).backgroundColor : null,
      slideMs: root.getPropertyValue('--page-slide-ms').trim(),
      wallpaper: document.querySelector('[data-testid="wallpaper"]')?.getAttribute('src') ?? null,
    }
  })
  check(
    '白天有壁纸时不再压白蒙版（照片本色显示）',
    lightLook.scrim === false && Boolean(lightLook.wallpaper),
    JSON.stringify(lightLook),
  )
  check(
    '有照片的标记挂在 <html> 上，玻璃面翻成白色半透明（深色文字仍可读）',
    lightLook.photo === '1' &&
      lightLook.barBg === 'rgba(255, 255, 255, 0.55)' &&
      lightLook.dotBg === 'rgba(255, 255, 255, 0.6)' &&
      // 这个用例里没有链接图块，有的话必须是同一档白玻璃
      (lightLook.tileBg === null || lightLook.tileBg === 'rgba(255, 255, 255, 0.55)'),
    JSON.stringify(lightLook),
  )
  check(
    '动画时长 300ms（比以前更从容）',
    // 压缩后的 CSS 会把它写成 `.3s`（历史上这里就被 260ms → `.26s` 坑过一次）
    ['.3s', '0.3s', '300ms'].includes(lightLook.slideMs),
    `--page-slide-ms=${lightLook.slideMs}`,
  )

  // 切回深色：蒙版该回来（照片上压黑才读得清）
  await page.locator('[data-testid="theme-toggle"]').click()
  await page.waitForTimeout(500)
  const darkLook = await page.evaluate(() => {
    const el = document.querySelector('[data-testid="wallpaper-scrim"]')
    const bar = document.querySelector('input[aria-label="搜索"]')?.closest('div')
    return {
      scrim: Boolean(el),
      scrimImage: el ? getComputedStyle(el).backgroundImage.slice(0, 30) : null,
      barBg: bar ? getComputedStyle(bar).backgroundColor : null,
      photo: document.documentElement.dataset.photo,
    }
  })
  check(
    '深色有照片时压黑蒙版仍在，玻璃回到深色淡染',
    darkLook.scrim === true &&
      darkLook.scrimImage?.includes('linear-gradient') &&
      darkLook.barBg === 'rgba(229, 237, 255, 0.1)' &&
      darkLook.photo === '1',
    JSON.stringify(darkLook),
  )

  check('浏览器控制台无 error', consoleErrors.length === 0, consoleErrors.slice(0, 2).join(' | '))
} catch (err) {
  check('用例执行未抛异常', false, err.message)
} finally {
  await browser.close()
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
if (failed.length) {
  console.log('失败项：')
  for (const f of failed) console.log(`  - ${f.name}${f.detail ? ' -> ' + f.detail : ''}`)
}
process.exit(failed.length ? 1 : 0)
