/**
 * M6 真浏览器验收：壁纸与设置中心
 *   1) 打开设置面板
 *   2) 上传壁纸 → 列表出现
 *   3) 设为全局 → 壁纸层出现真实图片（img[data-testid=wallpaper]）
 *   4) 轮换开关持久化
 *   5) 删除壁纸 → 回落到纯色兜底（data-testid=wallpaper-fallback）
 *   6) 新增自定义搜索引擎 → 出现在列表且能设为默认
 *   7) 交互设置（合并阈值）改完持久化，刷新后仍在
 */
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18090'

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}

// 生成一张 800x600 的纯色 PNG（用 canvas 在浏览器里造，省得在 node 里手写 PNG）
const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })
const consoleErrors = []
page.on('console', (m) => {
  if (m.type() === 'error') consoleErrors.push(m.text())
})
page.on('pageerror', (e) => consoleErrors.push('pageerror: ' + e.message.split('\n')[0]))

const settings = async () => (await fetch(`${BASE}/api/settings`)).json()
const wallpapers = async () => (await (await fetch(`${BASE}/api/wallpapers`)).json()).wallpapers

async function makePng(width, height) {
  return page.evaluate(
    ([w, h]) => {
      const canvas = document.createElement('canvas')
      canvas.width = w
      canvas.height = h
      const ctx = canvas.getContext('2d')
      const grad = ctx.createLinearGradient(0, 0, w, h)
      grad.addColorStop(0, '#123456')
      grad.addColorStop(1, '#fedcba')
      ctx.fillStyle = grad
      ctx.fillRect(0, 0, w, h)
      const dataUrl = canvas.toDataURL('image/png')
      return dataUrl.split(',')[1]
    },
    [width, height],
  )
}

try {
  await page.goto(BASE, { waitUntil: 'networkidle' })

  // ---- 1) 设置入口 ----
  await page.getByRole('button', { name: '打开设置' }).click()
  const dialog = page.locator('div[role="dialog"][aria-label="设置"]')
  await dialog.waitFor({ timeout: 5_000 })
  check('设置面板可打开', await dialog.isVisible())
  await page.screenshot({ path: 'e2e/shot-m6-01-settings.png' })

  // ---- 2) 上传壁纸 ----
  await dialog.getByRole('tab', { name: '壁纸' }).click()
  const png = await makePng(800, 600)
  await dialog.locator('input[aria-label="上传壁纸"]').setInputFiles({
    name: 'wall.png',
    mimeType: 'image/png',
    buffer: Buffer.from(png, 'base64'),
  })
  await page.waitForTimeout(1500)

  let list = await wallpapers()
  check('壁纸上传成功', list.length === 1 && list[0].kind === 'upload', `count=${list.length} kind=${list[0]?.kind}`)
  check('生成了 1920 以内的缩略图', Boolean(list[0]?.thumb_file), `thumb=${list[0]?.thumb_file}`)
  await page.screenshot({ path: 'e2e/shot-m6-02-wallpaper-list.png' })

  // ---- 3) 设为全局 ----
  await dialog.getByRole('button', { name: '设为全局' }).first().click()
  await page.waitForTimeout(800)
  const shown = page.locator('img[data-testid="wallpaper"]')
  await shown.waitFor({ timeout: 5_000 })
  const src = await shown.getAttribute('src')
  check('壁纸层显示真实图片', Boolean(src?.startsWith('/wallpapers/orig/')), `src=${src}`)

  const saved = await settings()
  check('全局壁纸写入设置', saved.wallpaper_id === list[0].id, `wallpaper_id=${saved.wallpaper_id}`)

  // ---- 4) 轮换 ----
  await dialog.getByRole('tab', { name: '外观' }).click()

  // ---- 图块形状预设（全局设置 → CSS 变量）----
  await dialog.getByTestId('tile-shape-select').selectOption('circle')
  await page.waitForTimeout(400)
  const shape = await page.evaluate(() => ({
    variable: getComputedStyle(document.documentElement).getPropertyValue('--radius-tile').trim(),
    big: getComputedStyle(document.documentElement).getPropertyValue('--radius-tile-lg').trim(),
  }))
  check(
    '圆形预设写进 --radius-tile（大文件块单独一档，避免 2×2 变正圆切内容）',
    shape.variable === '50%' && shape.big === '26%',
    JSON.stringify(shape),
  )
  check('形状写入服务端设置', (await settings()).tile_shape === 'circle', (await settings()).tile_shape)
  await dialog.getByTestId('tile-shape-select').selectOption('squircle')
  await page.waitForTimeout(300)
  check(
    '换成超椭圆',
    (await settings()).tile_shape === 'squircle' &&
      (await page.evaluate(() =>
        getComputedStyle(document.documentElement).getPropertyValue('--radius-tile').trim(),
      )) === '22%',
  )
  await dialog.getByTestId('tile-shape-select').selectOption('rounded')
  await page.waitForTimeout(300)

  await dialog.getByLabel('轮换').selectOption('load')
  await page.waitForTimeout(600)
  const rotated = await settings()
  check('轮换设置持久化', rotated.wallpaper_rotation === 'load', `rotation=${rotated.wallpaper_rotation}`)

  // ---- 5) 删除壁纸 → 兜底底色 ----
  await dialog.getByRole('tab', { name: '壁纸' }).click()
  await dialog.getByRole('button', { name: '删除壁纸' }).first().click()
  await page.waitForTimeout(1200)
  check('壁纸已删除', (await wallpapers()).length === 0)
  await page.locator('div[data-testid="wallpaper-fallback"]').waitFor({ timeout: 5_000 })
  check('无壁纸时回落到纯色兜底', true)
  const afterDelete = await settings()
  check('删除后自动清空全局引用', !afterDelete.wallpaper_id, `wallpaper_id=${afterDelete.wallpaper_id}`)

  // ---- 6) 自定义搜索引擎 ----
  await dialog.getByRole('tab', { name: '搜索' }).click()
  await dialog.getByRole('button', { name: '新增' }).click()
  await dialog.getByLabel('引擎名称').fill('Kagi')
  await dialog.getByLabel('引擎图标文字').fill('K')
  await dialog.getByLabel('引擎 URL 模板').fill('https://kagi.com/search?q={query}')
  await dialog.getByRole('button', { name: '保存' }).click()
  await page.waitForTimeout(900)

  const engines = (await (await fetch(`${BASE}/api/engines`)).json()).engines
  const kagi = engines.find((e) => e.name === 'Kagi')
  check('新增自定义引擎', Boolean(kagi), `engines=${engines.length}`)

  await dialog.getByLabel('默认搜索引擎').selectOption(kagi.id)
  await page.waitForTimeout(600)
  check('设为默认引擎', (await settings()).default_engine_id === kagi.id)

  // 内置引擎不能删
  const deleteBuiltin = dialog.getByRole('button', { name: '删除 Google' })
  check('内置引擎的删除按钮被禁用', await deleteBuiltin.isDisabled())
  await page.screenshot({ path: 'e2e/shot-m6-03-engines.png' })

  // ---- 7) 交互设置 ----
  await dialog.getByRole('tab', { name: '交互' }).click()
  await dialog.getByLabel('合并悬停阈值').fill('900')
  await page.waitForTimeout(600)
  check('合并阈值持久化', (await settings()).merge_dwell_ms === '900', `merge_dwell_ms=${(await settings()).merge_dwell_ms}`)

  await dialog.getByRole('button', { name: '关闭设置' }).click()
  await page.reload({ waitUntil: 'networkidle' })
  await page.waitForTimeout(600)
  const after = await settings()
  check('刷新后设置仍在', after.default_engine_id === kagi.id && after.merge_dwell_ms === '900')

  check('浏览器控制台无 error', consoleErrors.length === 0, consoleErrors.slice(0, 2).join(' | '))
} catch (err) {
  check('执行过程未抛异常', false, err.message)
  await page.screenshot({ path: 'e2e/shot-m6-fail.png' }).catch(() => {})
} finally {
  await browser.close()
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
process.exit(failed.length === 0 ? 0 : 1)
