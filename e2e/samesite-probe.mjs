// 「从 my_nav 点 DSH 的带 token 链接为什么 401」的根因证据脚本（结论见 docs/spec.md §11.4）。
//
// 干的事：从站点 A 点一个链接跳到站点 B 的 /?token=xxx，B 回 303 + Set-Cookie(SameSite=?)
// 并跳到 /，看浏览器到底带不带这条 cookie。六个场景：
//   [A] 跨站点击（同标签）        [B] 跨站点击 + target=_blank（my_nav 图块就是这个）
//   [C] 地址栏直接访问            [D] 先跨站失败、再按浏览器发起访问
//   [E] 已有 cookie 时再跨站点击  [F] **同 site 跨 origin**（"挂到同域名就能用"的依据）
//
// 跑法：PLAYWRIGHT_BROWSERS_PATH=./e2e/browsers node e2e/samesite-probe.mjs
//       SAMESITE=Lax node e2e/samesite-probe.mjs   # 换 cookie 属性做对比
//
// 为什么必须用真浏览器：curl 的 cookie jar **完全不实现 SameSite**，同一个 URL 用
// `curl -c/-b` 验永远是 200 —— 第一轮就是这么被骗过去的，别再踩。
//
// 站点隔离：http://127.0.0.1 与 http://localhost 是两个不同的 site
// （IP 与主机名各自成 site，端口不参与 site 判定），所以必须换主机名；
// [F] 用 --host-resolver-rules 把 a/b.example.com 都指到 127.0.0.1 造"同 site"。
import http from 'node:http'
import { chromium } from 'playwright'

const PORT_A = 18901
const PORT_B = 18902
const A = `http://127.0.0.1:${PORT_A}`
const B = `http://localhost:${PORT_B}`

const serverB = http.createServer((req, res) => {
  const u = new URL(req.url, B)
  if (u.pathname === '/') {
    const authed = (req.headers.cookie || '').includes('tok=')
    res.writeHead(authed ? 200 : 401, { 'Content-Type': 'text/plain' })
    res.end(authed ? 'AUTHED' : 'NO-COOKIE-401')
    return
  }
  if (u.pathname === '/token') {
    res.writeHead(303, {
      'Set-Cookie': `tok=1; Path=/; HttpOnly; SameSite=${process.env.SAMESITE || 'Strict'}`,
      Location: '/',
      'Cache-Control': 'no-store',
    })
    res.end()
    return
  }
  res.writeHead(404)
  res.end()
})

const serverA = http.createServer((req, res) => {
  const u = new URL(req.url, A)
  // ?to=<host> 决定链接指向哪个站点 B（默认 localhost；[F] 用 b.example.com 造"同 site 跨 origin"）
  const target = u.searchParams.get('to') || 'localhost'
  const href = `http://${target}:${PORT_B}/token?token=good`
  res.writeHead(200, { 'Content-Type': 'text/html' })
  res.end(`<a id="same" href="${href}">same tab</a>
           <a id="blank" href="${href}" target="_blank" rel="noreferrer noopener">new tab</a>`)
})

await new Promise((r) => serverA.listen(PORT_A, '127.0.0.1', r))
await new Promise((r) => serverB.listen(PORT_B, '127.0.0.1', r))

const browser = await chromium.launch({ args: ['--host-resolver-rules=MAP a.example.com 127.0.0.1,MAP b.example.com 127.0.0.1'] })
const ctx = await browser.newContext()
const page = await ctx.newPage()

async function probe(label, how) {
  await ctx.clearCookies()
  const before = await how()
  console.log(`${label}\n   最终 URL: ${before.url}\n   最终内容: ${before.body}\n   cookie: ${before.cookies}`)
}

// 1) 跨站点击（= my_nav 图块那种：A 的页面里点链接去 B）
await probe('[A] 从 A 点击链接（同标签）', async () => {
  await page.goto(A)
  await page.click('#same')
  await page.waitForLoadState()
  return { url: page.url(), body: (await page.textContent('body')).trim(), cookies: JSON.stringify(await ctx.cookies(B)) }
})

// 2) 跨站点击 + target=_blank（my_nav 实际用的就是这个）
await probe('[B] 从 A 点击链接（新标签 target=_blank）', async () => {
  await page.goto(A)
  const [popup] = await Promise.all([ctx.waitForEvent('page'), page.click('#blank')])
  await popup.waitForLoadState()
  return { url: popup.url(), body: (await popup.textContent('body')).trim(), cookies: JSON.stringify(await ctx.cookies(B)) }
})

// 3) 地址栏直接敲（浏览器发起，day-one 场景）
await probe('[C] 地址栏直接访问（浏览器发起）', async () => {
  await page.goto(`${B}/token?token=good`)
  return { url: page.url(), body: (await page.textContent('body')).trim(), cookies: JSON.stringify(await ctx.cookies(B)) }
})

// 4) 先跨站点过一回再去访问（cookie 到底有没有被种上？）
await probe('[D] 先跨站点击失败，再按浏览器发起访问', async () => {
  await page.goto(A)
  await page.click('#same')
  await page.waitForLoadState()
  const mid = { url: page.url(), body: (await page.textContent('body')).trim(), cookies: JSON.stringify(await ctx.cookies(B)) }
  await page.goto(B + '/')
  return { url: page.url(), body: `${mid.body} → 再访问后: ${(await page.textContent('body')).trim()}`, cookies: JSON.stringify(await ctx.cookies(B)) }
})

// 5) cookie 已经存在时，跨站点击还能不能用？（Strict 是否"永远"挡住）
await probe('[E] 先拿到 cookie，再从 A 点击不带 token 的 /', async () => {
  await page.goto(B + '/token?token=good') // 浏览器发起 → 种上 cookie
  await page.goto(A)
  await page.click('#same')
  await page.waitForLoadState()
  return { url: page.url(), body: (await page.textContent('body')).trim(), cookies: JSON.stringify(await ctx.cookies(B)) }
})

// 6) 同 site、跨 origin：a.example.com → b.example.com（用 host-resolver-rules 把两个
//    名字都指到 127.0.0.1）。这正是 http(s)://armbian-1.tailbae726.ts.net 与
//    https://debian-nayun.tailbae726.ts.net 的关系 —— 同一个 registrable domain。
//    若这里 Strict 也能通过，那"把 my_nav 挂到 tailnet HTTPS 上"就是零改 DSH 的解。
{
  const ctx2 = await browser.newContext()
  const page2 = await ctx2.newPage()
  await ctx2.clearCookies()
  await page2.goto(`http://a.example.com:${PORT_A}/?to=b.example.com`)
  await page2.click('#same')
  await page2.waitForLoadState()
  console.log(`[F] 同 site 跨 origin（a.example.com → b.example.com），SAMESITE=${process.env.SAMESITE || 'Strict'}
   最终 URL: ${page2.url()}
   最终内容: ${(await page2.textContent('body')).trim()}`)
  await ctx2.close()
}

await browser.close()
serverA.close()
serverB.close()
