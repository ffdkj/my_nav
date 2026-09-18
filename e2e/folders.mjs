/**
 * M3 真浏览器验收：文件夹全链路
 *   1) 拖到另一块上**悬停达阈值**后松手 → 合并成文件夹（并断言悬停期间出现"待合并"高亮）
 *   2) 小夹外显 9 个缩略位
 *   3) 点小夹 → 打开模态（role=dialog）
 *   4) 模态内"移出到主网格" → 夹内少一个、主网格多一个
 *   5) 编辑夹 → 切 2×2 → 网格上该块横跨 2 列（占 4 格）
 *   6) 大夹内部的图标是真实 <a href>，可直接点击
 *   7) 大夹空白处（内边距/缝）→ 打开与小夹同一个预览模态（Q36 决策 1）
 *   8) 预览模态内每个图标有"编辑图标"→ 复用普通图块的编辑对话框（候选卡片）
 *   9) 在里面手选一张候选 → 真的落到夹内那个图标上，且模态仍开着（复用闭环）
 */
import { createServer } from 'node:http'
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18090'
const BOARD_URL = `${BASE}/api/pages/page-home/board`

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}

async function board() {
  return (await fetch(BOARD_URL)).json()
}
const folderItems = (b) => b.items.filter((i) => i.kind === 'folder')
const linkItems = (b) => b.items.filter((i) => i.kind === 'link')

// ---- 离线小站点：给"夹内改图标"提供确定性候选（不依赖外网） ----
let sitePng = Buffer.alloc(0)
const site = createServer((req, res) => {
  if (req.url === '/icon.png' || req.url === '/favicon.ico') {
    res.writeHead(200, { 'Content-Type': 'image/png' })
    res.end(sitePng)
    return
  }
  res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' })
  res.end(
    `<!doctype html><html><head><link rel="icon" sizes="256x256" href="/icon.png"></head><body>stub</body></html>`,
  )
})
await new Promise((resolve) => site.listen(0, '127.0.0.1', resolve))
const siteURL = `http://127.0.0.1:${site.address().port}/`

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
sitePng = Buffer.from(await page.evaluate(makePngInPage, [256, 256, '#22c55e', '#0ea5e9']), 'base64')
const consoleErrors = []
page.on('console', (m) => {
  if (m.type() === 'error') consoleErrors.push(m.text())
})
page.on('pageerror', (e) => consoleErrors.push('pageerror: ' + e.message.split('\n')[0]))

try {
  // 本脚本验证合并，明确要求阈值 500ms（并顺带验证了设置项真的生效）
  await fetch(`${BASE}/api/settings`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ merge_dwell_ms: '500' }),
  }).catch(() => {})
  const dwell = await (await fetch(`${BASE}/api/settings`)).json()
  check('合并阈值设置为 500ms', dwell.merge_dwell_ms === '500', `merge_dwell_ms=${dwell.merge_dwell_ms}`)

  await page.goto(BASE, { waitUntil: 'networkidle' })

  // 准备两个链接
  const addTile = page.getByRole('button', { name: '添加图标' })
  for (const s of [
    { url: 'https://github.com', title: 'GitHub' },
    { url: 'https://gitee.com', title: 'Gitee' },
  ]) {
    await addTile.click()
    await page.fill('#link-url-input', s.url)
    await page.getByPlaceholder('例如 GitHub').fill(s.title)
    await page.getByRole('button', { name: '确定' }).click()
    await page.getByRole('link', { name: new RegExp(s.title) }).waitFor({ timeout: 10_000 })
  }

  // ---- 1) 悬停 dwell 合并 ----
  const a = await page.locator('a[href="https://github.com"]').boundingBox()
  const b = await page.locator('a[href="https://gitee.com"]').boundingBox()
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2)
  await page.mouse.down()
  await page.mouse.move(a.x + a.width / 2 + 14, a.y + a.height / 2 + 6, { steps: 5 })
  await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 20 })
  // 阈值默认 500ms；停住 800ms 让它武装
  await page.waitForTimeout(800)
  const armedCount = await page.locator('.scale-105').count()
  check('悬停达阈值出现「待合并」高亮', armedCount > 0, `高亮元素=${armedCount}`)
  await page.screenshot({ path: 'e2e/shot-m3-01-armed.png' })

  await page.mouse.up()
  await page.waitForTimeout(900)

  await waitSaved(page)
  let bd = await board()
  const folders = folderItems(bd)
  check('合并后生成 1 个文件夹', folders.length === 1, `folders=${folders.length}`)
  check(
    '文件夹内含 2 个图标',
    folders[0]?.children?.length === 2,
    `children=${folders[0]?.children?.length}`,
  )
  check('页面上不再有裸链接块', linkItems(bd).length === 0, `links=${linkItems(bd).length}`)
  await page.screenshot({ path: 'e2e/shot-m3-02-folder.png' })

  // ---- 2) 小夹外显 9 个缩略位 ----
  // 限定在网格内：设置齿轮的 aria-label 也以「打开」开头，全局匹配会撞名
  const slots = await page.locator('ul[aria-label="导航图标"] button[aria-label^="打开"] .grid > *').count()
  check('小夹外显 9 个缩略位', slots === 9, `slots=${slots}`)

  // ---- 3) 点击打开模态 ----
  await page.locator('ul[aria-label="导航图标"] button[aria-label^="打开"]').click()
  const modal = page.locator('div[role="dialog"]')
  await modal.waitFor({ timeout: 5_000 })
  check('点击小夹打开模态', await modal.isVisible())
  await page.screenshot({ path: 'e2e/shot-m3-03-modal.png' })

  // ---- 4) 移出到主网格 ----
  await page.locator('button[aria-label="移出到主网格"]').first().click()
  await page.waitForTimeout(700)
  await waitSaved(page)
  bd = await board()
  check(
    '移出后夹内剩 1 个、主网格多 1 个',
    folderItems(bd)[0]?.children?.length === 1 && linkItems(bd).length === 1,
    `children=${folderItems(bd)[0]?.children?.length} links=${linkItems(bd).length}`,
  )
  await page.keyboard.press('Escape')
  await page.locator('button[aria-label="关闭"]').click().catch(() => {})
  await page.waitForTimeout(300)

  // ---- 5) 切成 2x2 大夹 ----
  await page.locator('ul[aria-label="导航图标"] button[aria-label^="编辑"]').first().click()
  const fdialog = page.locator('div[role="dialog"][aria-label="编辑文件夹"]')
  await fdialog.waitFor({ timeout: 5_000 })
  await fdialog.getByRole('button', { name: /2×2/ }).click()
  await fdialog.getByRole('button', { name: '确定' }).click()
  await page.waitForTimeout(800)

  bd = await board()
  const size = folderItems(bd)[0]?.size
  check('文件夹尺寸变为 2', size === 2, `size=${size}`)

  const span = await page
    .locator('ul[aria-label="导航图标"] > li')
    .first()
    .evaluate((el) => getComputedStyle(el).gridColumn)
  check('大夹在网格横跨 2 列（占 4 格）', /span 2/.test(span), `grid-column=${span}`)
  await page.screenshot({ path: 'e2e/shot-m3-04-big.png' })

  // ---- 6) 大夹内部图标可直接点击 ----
  const innerLinks = await page.locator('ul[aria-label="导航图标"] > li').first().locator('a[href]').count()
  check('大夹内部图标是真实链接', innerLinks >= 1, `a[href]=${innerLinks}`)

  // ---- 7) 大夹空白处 = 展开同一个预览模态 ----
  // 直接用 elementFromPoint 判定"谁在最上面"：图标中心必须是 <a>（点了就跳转），
  // 内边距必须是展开按钮 —— 这条断言不依赖点击后跳走，headless 下也能验。
  const bigTile = page.locator('ul[aria-label="导航图标"] > li').first()
  const hits = await bigTile.evaluate((li) => {
    const probe = (x, y) => {
      const el = document.elementFromPoint(x, y)
      if (!el) return 'none'
      if (el.closest('a[href]')) return 'link'
      if (el.closest('[data-testid="bigfolder-open"]')) return 'open'
      return el.tagName.toLowerCase()
    }
    const a = li.querySelector('a[href]').getBoundingClientRect()
    const btn = li.querySelector('[data-testid="bigfolder-open"]').getBoundingClientRect()
    return {
      icon: probe(a.left + a.width / 2, a.top + a.height / 2),
      corner: probe(btn.left + 4, btn.top + 4),
    }
  })
  check('大夹内图标中心仍命中链接（没被展开层抢走）', hits.icon === 'link', `hit=${hits.icon}`)
  check('大夹内边距命中展开按钮', hits.corner === 'open', `hit=${hits.corner}`)

  // 悬停空白处要有可见提示（环 + 淡底），否则"这里能点"根本发现不了。
  // 断言读 --tw-ring-color 而不是 box-shadow：自定义属性不参与 transition，
  // 不会撞上 headless 下 transition 取值卡在起点的问题。
  const hint = page.locator('[data-testid="bigfolder-open"] > span')
  const ringBefore = await hint.evaluate((el) => getComputedStyle(el).getPropertyValue('--tw-ring-color').trim())
  await page.locator('[data-testid="bigfolder-open"]').hover({ position: { x: 4, y: 4 } })
  await page.waitForTimeout(250)
  const ringAfter = await hint.evaluate((el) => getComputedStyle(el).getPropertyValue('--tw-ring-color').trim())
  check('悬停空白处出现可见提示（展开环变色）', ringBefore !== ringAfter, `${ringBefore} -> ${ringAfter}`)
  await page.screenshot({ path: 'e2e/shot-m3-05-big-hover.png' })

  // 点左上角内边距（按钮中心被图标盖着，Playwright 会判定被拦截）
  await page.locator('[data-testid="bigfolder-open"]').click({ position: { x: 4, y: 4 } })
  const fmodal = page.locator('[data-testid="folder-modal"]')
  await fmodal.waitFor({ timeout: 5_000 })
  check('点大夹空白处打开预览模态（复用小夹那个）', await fmodal.isVisible())
  await page.screenshot({ path: 'e2e/shot-m3-06-big-preview.png' })

  // ---- 8) 模态内改夹内图标的图标：复用普通图块的编辑对话框 ----
  const childId = folderItems(await board())[0].children[0].link_id
  // 把夹内这个链接指到离线小站点，候选才是确定的（不赌外网）
  await fetch(`${BASE}/api/links/${childId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url: siteURL, title: 'Stub' }),
  })
  await page.reload({ waitUntil: 'networkidle' })
  await page.locator('[data-testid="bigfolder-open"]').click({ position: { x: 4, y: 4 } })
  await fmodal.waitFor({ timeout: 5_000 })

  const pencil = fmodal.locator('[data-testid="folder-link-edit"]')
  check('预览模态内每个图标都有「编辑图标」按钮', (await pencil.count()) === 1, `count=${await pencil.count()}`)
  await pencil.first().hover()
  await page.waitForTimeout(200)
  await page.screenshot({ path: 'e2e/shot-m3-07-folder-link-edit.png' })
  await pencil.first().click()
  const dialog = page.locator('form[aria-label="编辑图标"]')
  await dialog.waitFor({ timeout: 5_000 })
  await page.locator('#link-url-input').waitFor({ timeout: 5_000 })
  check(
    '「编辑图标」打开的是普通链接编辑对话框（地址已带出）',
    (await page.inputValue('#link-url-input')) === siteURL,
    await page.inputValue('#link-url-input'),
  )

  const cards = dialog.locator('[data-testid="icon-card-candidate"]')
  await cards.first().waitFor({ timeout: 20_000 })
  check('对话框里列出了候选卡片', (await cards.count()) >= 1, `count=${await cards.count()}`)
  await cards.first().click()
  await dialog.getByRole('button', { name: '确定' }).click()
  await page.waitForTimeout(1200)
  await waitSaved(page)

  const after = (await board()).links.find((l) => l.id === childId)
  check(
    '手选候选真的落到夹内那个图标上（icon_picked_url 已记下）',
    Boolean(after?.icon_picked_url) && after?.icon_status === 'ok',
    `picked=${after?.icon_picked_url ? 'yes' : 'no'} status=${after?.icon_status}`,
  )
  check('改完图标预览模态仍然开着（复用闭环）', await fmodal.isVisible())
  await page.screenshot({ path: 'e2e/shot-m3-08-modal-icon-picked.png' })

  // 补一张：首屏（大夹在网格上）的空白处提示，给人工复核留证据
  await fmodal.locator('button[aria-label="关闭"]').click()
  await page.locator('[data-testid="bigfolder-open"]').hover({ position: { x: 4, y: 4 } })
  await page.waitForTimeout(250)
  await page.screenshot({ path: 'e2e/shot-m3-09-big-on-grid.png' })

  check('浏览器控制台无 error', consoleErrors.length === 0, consoleErrors.slice(0, 2).join(' | '))
} catch (err) {
  check('执行过程未抛异常', false, err.message)
  await page.screenshot({ path: 'e2e/shot-m3-fail.png' }).catch(() => {})
} finally {
  await browser.close()
  site.close()
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
process.exit(failed.length === 0 ? 0 : 1)
