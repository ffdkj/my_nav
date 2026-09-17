/**
 * M7 真浏览器验收：导出 / 导入 / 备份
 *   1) 通过设置面板下载 JSON（Playwright 接住 download）
 *   2) 下载 zip（含资源）
 *   3) 改一个页面名 → 导入刚才的 JSON → 名字回到导出时的状态（全量覆盖生效）
 *   4) 导入后出现"导入前快照"，且能下载
 *   5) 连续导入两次，服务器上始终只有一份快照（固定文件名）
 *   6) 坏数据（网格重叠）导入被拒 → 现状不变，且不写快照
 */
import { readFileSync, existsSync, readdirSync } from 'node:fs'
import { chromium } from 'playwright'

const BASE = process.env.NAV_BASE ?? 'http://127.0.0.1:18090'
const DATA_DIR = process.env.NAV_DATA_DIR ?? '.smoke/transfer'

const results = []
function check(name, ok, detail = '') {
  results.push({ name, ok, detail })
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? '  -> ' + detail : ''}`)
}

const api = {
  pages: async () => (await (await fetch(`${BASE}/api/pages/`)).json()).pages,
  backup: async () => (await fetch(`${BASE}/api/backup`)).json(),
}

const browser = await chromium.launch()
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })
const consoleErrors = []
page.on('console', (m) => {
  if (m.type() === 'error') consoleErrors.push(m.text())
})
page.on('pageerror', (e) => consoleErrors.push('pageerror: ' + e.message.split('\n')[0]))

async function openDataTab() {
  const dialog = page.locator('div[role="dialog"][aria-label="设置"]')
  // 幂等：面板可能已经开着（导入后页面会重载，所以这里两种状态都要容错）。
  // 不复用这层判断的话，第二次点齿轮会被模态遮罩挡住而超时。
  if ((await dialog.count()) === 0 || !(await dialog.isVisible())) {
    await page.getByRole('button', { name: '打开设置' }).click()
    await dialog.waitFor({ timeout: 5_000 })
  }
  await dialog.getByRole('tab', { name: '数据' }).click()
  return dialog
}

async function download(href) {
  const [dl] = await Promise.all([
    page.waitForEvent('download', { timeout: 20_000 }),
    page.locator(`a[href="${href}"]`).click(),
  ])
  const path = await dl.path()
  return { name: dl.suggestedFilename(), path, body: path ? readFileSync(path) : Buffer.alloc(0) }
}

try {
  await page.goto(BASE, { waitUntil: 'networkidle' })
  await page.getByRole('button', { name: '添加图标' }).click()
  await page.fill('#link-url-input', 'https://example.com')
  await page.getByPlaceholder('例如 GitHub').fill('Example')
  await page.getByRole('button', { name: '确定' }).click()
  await page.waitForFunction(() => !document.body.innerText.includes('保存中'), null, {
    timeout: 20_000,
  })

  let dialog = await openDataTab()

  // ---- 1) 导出 JSON ----
  const json = await download('/api/export')
  check('能下载 JSON 导出', json.name.endsWith('.json') && json.body.length > 50, `name=${json.name}`)
  let doc = JSON.parse(json.body.toString('utf8'))
  check('导出内容含页面与链接', doc.pages?.length >= 1 && doc.links?.length === 1, `pages=${doc.pages?.length} links=${doc.links?.length}`)
  check('导出文档带版本号', doc.version === 1, `version=${doc.version}`)

  // ---- 2) 导出 zip ----
  const zip = await download('/api/export?withAssets=1')
  const isZip = zip.body[0] === 0x50 && zip.body[1] === 0x4b
  check('能下载 zip 导出', zip.name.endsWith('.zip') && isZip, `name=${zip.name} size=${zip.body.length}`)

  // ---- 3) 改名 → 导入 JSON → 回到导出时的状态 ----
  const before = await api.pages()
  const homeId = before[0].id
  await fetch(`${BASE}/api/pages/${homeId}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: '被改坏的名字' }),
  })
  check('改名已生效', (await api.pages())[0].name === '被改坏的名字')

  page.once('dialog', (d) => d.accept())
  await dialog.locator('input[aria-label="导入文件"]').setInputFiles({
    name: json.name,
    mimeType: 'application/json',
    buffer: json.body,
  })
  await page.waitForLoadState('networkidle')
  await page.waitForTimeout(1500)

  const afterImport = await api.pages()
  check(
    '导入是覆盖式：名字回到导出时的值',
    afterImport[0].name === before[0].name,
    `name=${afterImport[0].name}`,
  )

  const board = await (await fetch(`${BASE}/api/pages/${afterImport[0].id}/board`)).json()
  check('图标也回来了', board.links.length === 1, `links=${board.links.length}`)
  await page.screenshot({ path: 'e2e/shot-m7-01-after-import.png' })

  // ---- 4) 快照出现且可下载 ----
  const backup = await api.backup()
  check('导入后生成快照', backup.exists === true && backup.bytes > 0, `at=${backup.at}`)

  dialog = await openDataTab()
  const backupLink = page.locator('a[href="/api/backup/download"]')
  check('设置面板显示快照并可下载', (await backupLink.count()) === 1)
  const backupFile = await download('/api/backup/download')
  const backupDoc = JSON.parse(backupFile.body.toString('utf8'))
  check(
    '快照内容是导入前的状态',
    backupDoc.pages?.[0]?.name === '被改坏的名字',
    `snapshot name=${backupDoc.pages?.[0]?.name}`,
  )

  // ---- 5) 再导入一次，快照仍只有一份 ----
  dialog = await openDataTab()
  page.once('dialog', (d) => d.accept())
  await dialog.locator('input[aria-label="导入文件"]').setInputFiles({
    name: json.name,
    mimeType: 'application/json',
    buffer: json.body,
  })
  await page.waitForLoadState('networkidle')
  await page.waitForTimeout(1200)

  const backupDir = `${DATA_DIR}/backup`
  const files = existsSync(backupDir) ? readdirSync(backupDir) : []
  check(
    '服务器上始终只有一份快照',
    files.length === 1 && files[0] === 'pre-import.json',
    `files=${JSON.stringify(files)}`,
  )

  // ---- 6) 坏数据被拒且不破坏现状 ----
  const broken = JSON.parse(JSON.stringify(doc))
  const items = broken.pages[0].items
  if (items.length >= 2) {
    items[1].col = items[0].col
    items[1].row = items[0].row
  } else {
    // 只有一个图标时，人为造一个越界坐标
    items[0].col = 99
  }
  const res = await fetch(`${BASE}/api/import`, { method: 'POST', body: (() => {
    const form = new FormData()
    form.append('file', new Blob([JSON.stringify(broken)], { type: 'application/json' }), 'broken.json')
    return form
  })() })
  check('坏数据导入被拒（422）', res.status === 422, `status=${res.status}`)

  const stillThere = await api.pages()
  check('被拒后数据未被破坏', stillThere.length === afterImport.length, `pages=${stillThere.length}`)

  // 校验失败不该写快照：把快照删掉再试一次，确认它没有被重建
  const snapshotBefore = (await api.backup()).at
  const res2 = await fetch(`${BASE}/api/import`, { method: 'POST', body: (() => {
    const form = new FormData()
    form.append('file', new Blob([JSON.stringify(broken)], { type: 'application/json' }), 'broken.json')
    return form
  })() })
  const snapshotAfter = (await api.backup()).at
  check(
    '校验失败不写快照',
    res2.status === 422 && snapshotBefore === snapshotAfter,
    `before=${snapshotBefore} after=${snapshotAfter}`,
  )

  check('浏览器控制台无 error', consoleErrors.length === 0, consoleErrors.slice(0, 2).join(' | '))
} catch (err) {
  check('执行过程未抛异常', false, err.message)
  await page.screenshot({ path: 'e2e/shot-m7-fail.png' }).catch(() => {})
} finally {
  await browser.close()
}

const failed = results.filter((r) => !r.ok)
console.log(`\n${results.length - failed.length}/${results.length} 通过`)
process.exit(failed.length === 0 ? 0 : 1)
