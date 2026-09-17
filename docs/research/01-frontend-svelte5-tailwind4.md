# Svelte 5 + Vite + Tailwind v4 + Lucide + vite-plugin-pwa — Grounded Research

**Target:** single-page personal navigation dashboard, Go backend on `:8080`.
**Method:** Context7 MCP primary; versions from **live npm `latest` dist-tags**; `svelte.dev/docs/cli/sv-create`
fetched; real `create-vite@9.2.1` tarball extracted to read the generated template.
⚠️ **`web_search` was unavailable (no API key)** — no web corroboration.

## 0. Versions (npm `latest`, live)

| Package | latest | Note |
|---|---|---|
| `svelte` | **5.57.0** | runes |
| `@sveltejs/vite-plugin-svelte` | **7.3.0** | peer `vite ^8`, `svelte ^5.46.4`; node `^20.19\|^22.12\|>=24` |
| `vite` | **8.3.0** | `previous` = 7.3.6 |
| `tailwindcss` / `@tailwindcss/vite` | **4.3.3** | vite peer `^5.2\|^6\|^7\|^8` |
| `@lucide/svelte` | **1.47.0** | peer `svelte: ^5` — **the Svelte 5 package** |
| `lucide-svelte` | 1.0.1 | **Svelte 4 package — do not use** |
| `vite-plugin-pwa` | **1.3.0** | vite peer `^3–^8`; bundles workbox `7.4.1` as real deps |
| `svelte-check` / `@tsconfig/svelte` | 4.7.6 / 5.0.8 | |
| `create-vite` / `sv` | 9.2.1 / 0.17.0 | `sv` = **SvelteKit only** |
| `typescript` | 7.0.2 | ⚠️ template pins `~6.0.2` — see §7 |

## 1. Scaffolding a Svelte 5 + TS + Vite SPA

**Path A — plain Vite SPA (recommended here):** `npm create vite@latest my-nav -- --template svelte-ts`
Verified: `create-vite@9.2.1` ships `template-svelte` and `template-svelte-ts`.

**Path B — SvelteKit:** `npx sv create` scaffolds **SvelteKit**, not a bare SPA (`--template` ∈
`minimal|demo|library`, all SvelteKit). Take it only if you want Kit routing (then `adapter-static` +
`ssr = false`). The task specifies a Vite SPA → Path A.

**Files generated:** `index.html`, `package.json`, `svelte.config.js`, `vite.config.ts`, `tsconfig.json` +
`tsconfig.app.json` + `tsconfig.node.json`, `src/{main.ts,App.svelte,app.css,lib/Counter.svelte,assets/*}`,
`public/{favicon.svg,icons.svg}`, `.vscode/extensions.json`, `_gitignore`.

`svelte.config.js` is **effectively empty** — TS needs no preprocessor config:
```js
/** @type {import("@sveltejs/vite-plugin-svelte").SvelteConfig} */
export default {}
```
`vite.config.ts`: `export default defineConfig({ plugins: [svelte()] })`

`src/main.ts` uses the **Svelte 5 `mount` API** (not `new App()`):
```ts
import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
const app = mount(App, { target: document.getElementById('app')! })
export default app
```
`tsconfig.json` is a project-references root (`files: []` → app + node). `tsconfig.app.json` extends
`@tsconfig/svelte/tsconfig.json`, `target: es2023`, `noEmit`, `types:["svelte","vite/client"]`,
`allowArbitraryExtensions`, `checkJs`. Scripts `dev`/`build`/`preview` plus
`check: svelte-check --tsconfig ./tsconfig.app.json && tsc -p tsconfig.node.json`.

## 2. Tailwind CSS v4 + Vite + Svelte

`npm install tailwindcss @tailwindcss/vite` — use the **Vite plugin**, not `@tailwindcss/postcss`:
```ts
import tailwindcss from '@tailwindcss/vite'
export default defineConfig({ plugins: [svelte(), tailwindcss()] })
```
`src/app.css` (already imported by `main.ts`) — **this CSS is the config; no `tailwind.config.js` exists**:
```css
@import "tailwindcss";
@theme {
  --color-midnight: #121063;   /* → bg-midnight / text-midnight */
  --color-tahiti:   #3ab7bf;
  --font-display:   "Inter", sans-serif;
}
```
Namespaces map to utilities: `--color-*`, `--font-*`, `--breakpoint-*`, `--radius-*`, `--shadow-*`.

**Dark mode for a theme toggle.** v4's default `dark:` is `prefers-color-scheme`. For a **class-toggled**
theme (what a dashboard toggle needs), override the variant — then put `class="dark"` on `<html>` and drive
it from Svelte state inside an `$effect`:
```css
@import "tailwindcss";
@custom-variant dark (&:where(.dark, .dark *));
```
Data-attribute alternative: `@custom-variant dark (&:where([data-theme=dark], [data-theme=dark] *));`

**v3 → v4 gotchas:**
- **No default border color** — `border` is now `currentColor` (was `gray-200`).
- **Rings are 1px** — use **`ring-3`** to keep v3's 3px look.
- **Removed deprecated utilities:** `text-opacity-*` → `text-{color}/*`, `flex-grow-*` → `grow-*`,
  `decoration-slice` → `box-decoration-slice`.
- **PostCSS plugin + CLI are separate packages** (`@tailwindcss/postcss`, `@tailwindcss/cli`).
- **`@apply` in separately-bundled CSS (Svelte `<style>` blocks, CSS modules) loses theme vars/custom
  utilities** → add `@reference "../app.css";` in the block, or better, use the CSS variables directly.
- **Browser floor:** Safari 16.4+, Chrome 111+, Firefox 128+. Upgrade tool `npx @tailwindcss/upgrade` (Node 20+).
- ⚠️ Unverified: whether `@config "./tailwind.config.js"` still works in 4.3.3. Assume CSS-first only.

## 3. Svelte 5 runes

**(a) Shared global state in a `.svelte.ts` module** — that extension is **required** or runes won't compile.
```ts
// src/lib/stores/nav.svelte.ts
class NavStore {
  links   = $state<Link[]>([])
  query   = $state('')
  visible = $derived(this.links.filter(l => l.title.includes(this.query)))
  add(l: Link) { this.links.push(l) }
}
export const nav = new NavStore()      // singleton, importable anywhere
```
- ✅ `export const counter = $state({ count: 0 })` is fine **if you only mutate properties**
  (`counter.count += 1`), never reassign the binding.
- ❌ `export let count = $state(0)` + `count += 1` **breaks** — the compiler handles one file at a time and
  cannot wrap cross-module access in getters/setters ("state cannot be exported from a module if it is
  reassigned"). Use the class above, or `let count = $state(0)` + exported `getCount()`/`increment()` fns.
- SSR caveat: module-level `$state` is a **shared singleton across requests** — irrelevant for a pure SPA;
  use `setContext`/`getContext` for per-tree state.

**(b) `$props()`** replaces `export let`; destructuring/defaults/rename/rest are plain JS:
```svelte
<script lang="ts">
  interface Props { title?: string; links: Link[]; onselect: (l: Link) => void }
  let { title = 'Nav', links, onselect }: Props = $props()
</script>
```
Don't mutate props directly (use `$bindable()` for two-way). Callback props (`onselect`) replace
`createEventDispatcher`.

**(c) `$effect` cleanup** — return a teardown fn; runs before each re-run **and** on destroy. Runs after
mount, browser-only. **Never derive state in `$effect`** — use `$derived` / `$derived.by(() => …)`.
```svelte
<script lang="ts">
  $effect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === '/') focusSearch() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })
</script>
```

**(d) Dynamic components + events.** `on:click` is **gone** — handlers are plain attributes:
```svelte
<button onclick={() => count++}>+1</button>
<input oninput={e => query = e.currentTarget.value} />
```
Components are dynamic by default; `<svelte:component>` is unnecessary:
```svelte
<script lang="ts">
  let tab = $state('a')
  const View = $derived(tab === 'a' ? A : B)   // or {#const View = cond ? A : B}
</script>
<View />
```

**(e) Snippets + `{@render}`** replace slots; `children` is the implicit default snippet. Parameterized type
is `Snippet<[Link]>`; generics via `<script lang="ts" generics="T">`.
```svelte
<!-- Card.svelte -->
<script lang="ts">
  import type { Snippet } from 'svelte'
  let { title, children }: { title: string; children: Snippet } = $props()
</script>
<h2>{title}</h2>{@render children()}
```
```svelte
{#snippet row(l)} <li>{l.title}</li> {/snippet}
<List data={links} {row} />
```

## 4. Lucide for Svelte

- **Use `@lucide/svelte`** (1.47.0, peer `svelte ^5`). Docs: *"The `@lucide/svelte` package is designed
  specifically for Svelte 5. Users working with Svelte 4 should utilize the `lucide-svelte` package
  instead."* → `lucide-svelte` (1.0.1) is the Svelte 4 line; **not** for this project.
- Barrel: `import { Trees } from '@lucide/svelte'`
- **Deep import (best tree-shaking / faster builds):** `import CircleAlert from '@lucide/svelte/icons/circle-alert'`
- Usage: `<CircleAlert color="#ff3e98" size={20} strokeWidth={2} />`. Library is tree-shakable — only
  imported icons ship; deep imports are the documented optimization. Lab icons: `Icon` + `@lucide/lab`.
- ⚠️ Package name + Svelte 5 mapping verified directly; individual prop names were not verified against
  `@lucide/svelte@1.47.0` specifically.

## 5. vite-plugin-pwa — SPA config

```ts
import { VitePWA } from 'vite-plugin-pwa'

VitePWA({
  registerType: 'autoUpdate',          // forces workbox clientsClaim + skipWaiting = true
  includeAssets: ['favicon.svg'],
  manifest: {
    name: 'My Nav', short_name: 'Nav', description: 'Personal start page',
    theme_color: '#0b0f19', background_color: '#0b0f19', display: 'standalone', start_url: '/',
    icons: [
      { src: 'pwa-192x192.png', sizes: '192x192', type: 'image/png' },
      { src: 'pwa-512x512.png', sizes: '512x512', type: 'image/png' },
      { src: 'pwa-512x512.png', sizes: '512x512', type: 'image/png', purpose: 'maskable' },
    ],
  },
  workbox: {
    globPatterns: ['**/*.{js,css,html,ico,png,svg,woff2}'],
    navigateFallback: 'index.html',     // SPA shell for client-side routes
    navigateFallbackDenylist: [/^\/api/],
    runtimeCaching: [
      { urlPattern: ({ url, request }) => url.pathname.startsWith('/api') && request.method === 'GET',
        handler: 'NetworkFirst',        // ← NOT CacheFirst / StaleWhileRevalidate for live nav data
        options: { cacheName: 'nav-api', networkTimeoutSeconds: 3,
                   expiration: { maxEntries: 50, maxAgeSeconds: 86400 },
                   cacheableResponse: { statuses: [0, 200] } } },
      { urlPattern: ({ url }) => url.pathname.startsWith('/api'),
        method: 'POST', handler: 'NetworkOnly' },   // never replay writes from cache
    ],
  },
  devOptions: { enabled: true, navigateFallbackAllowlist: [/^\/index\.html$/] },
})
```
**Gotchas:**
- **Caching API responses is the classic stale-data bug.** `CacheFirst`/`StaleWhileRevalidate` on `/api`
  serves yesterday's links and looks like a backend bug. Use **`NetworkFirst`** with
  `networkTimeoutSeconds`, or `NetworkOnly` (+ `backgroundSync` for POSTs). Always set `expiration`.
- `'autoUpdate'` activates the new SW immediately — a **precached old app shell** can be served; it reloads
  the page for you, but in-flight state is lost. Use `'prompt'` if you want to control the reload.
- In dev the SW intercepts routes and breaks HMR — keep `devOptions.enabled` off by default; set
  `navigateFallbackAllowlist` when you turn it on. Exclude backend paths via `navigateFallbackDenylist`.
- Don't precache large assets (`globPatterns`) — precache reruns on every install.
- Only `@vite-pwa/assets-generator` is an *optional* peer; workbox is bundled.

## 6. Vite dev proxy → Go backend on :8080

```ts
export default defineConfig({
  plugins: [svelte(), tailwindcss(), VitePWA({ /* … */ })],
  server: {
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: true },
      // Add rewrite ONLY if Go handlers do NOT expect the /api prefix:
      // rewrite: (path) => path.replace(/^\/api/, ''),
    },
  },
})
```
- `changeOrigin: true` rewrites the `Host` header — needed for most Go/vhost setups.
- **Do not** add `rewrite` blindly: if Go registers `/api/...` routes (`mux.HandleFunc("/api/links")`),
  stripping the prefix 404s.
- Proxy is **dev-only**. In production serve the built SPA same-origin with Go (`go:embed` of `dist/`, or
  nginx/Caddy in front) — otherwise you need CORS.
- Vite 8 detail: the `configure(proxy, options)` hook now receives an **`http-proxy-3`** instance.
- WebSockets need `ws: true`; `rewriteWsOrigin` carries a documented CSRF risk.

## 7. Deprecations / breaking changes

**Svelte 4 → 5:** `export let` → `$props()`; `on:click`/`on:input` → `onclick`/`oninput` (plain attributes;
component event forwarding via `on:` gone); `createEventDispatcher` → callback props; `<slot>` → snippets +
`{@render}`; `$$slots`/`$$props`/`$$restProps` → `$props()`; `<svelte:component this={X}>` → `<X />` (use
`{@const View = …}` or `$derived` for expressions); `new App({target})` → `mount(App, {target})`; stores
still work but runes are the direction; **`.svelte.ts`/`.svelte.js` extension required** for runes modules.

**Tailwind v3 → v4:** §2 (no default border color, 1px rings, removed utility aliases, split PostCSS/CLI
packages, `@reference` for `@apply` in Svelte `<style>`, browser floor).

**Toolchain:**
- `@sveltejs/vite-plugin-svelte@7` requires **Vite ^8** + Svelte ^5.46.4, Node `^20.19||^22.12||>=24`.
  `vite-plugin-pwa@1.3.0` and `@tailwindcss/vite@4.3.3` both support Vite ^8 → the stack is mutually
  compatible on Vite 8.
- ⚠️ **TypeScript discrepancy — needs a parent decision:** npm `latest` is **7.0.2**, but `create-vite`'s
  `svelte-ts` template pins `typescript: ~6.0.2`. I did not verify which is right for this stack. Safest is
  to follow the template (`~6.0.2`, known-good) unless you deliberately adopt 7.0.2 after checking
  `svelte-check` 4.7.6 compatibility.
- `npx sv create` = SvelteKit only; a bare Vite SPA must come from `create-vite`.

## 8. Sources

**Context7 IDs:** `/websites/svelte_dev` (runes, `$props`, `$effect`, snippets, `$state` module sharing, v5
migration guide) · `/tailwindlabs/tailwindcss.com` (dark mode, upgrade guide, v3→v4 changes) ·
`/websites/lucide_dev` (package mapping, deep-import tree-shaking) · `/vite-pwa/docs` +
`/vite-pwa/vite-plugin-pwa` (`registerType`, manifest, `runtimeCaching`, `devOptions`) · `/websites/vite_dev`
(`server.proxy`).

**URLs:** [sv-create](https://svelte.dev/docs/cli/sv-create) ·
[$state](https://svelte.dev/docs/svelte/%24state) ·
[v5-migration-guide](https://svelte.dev/docs/svelte/v5-migration-guide) ·
[dark-mode](https://tailwindcss.com/docs/dark-mode) ·
[upgrade-guide](https://tailwindcss.com/docs/upgrade-guide) ·
[compatibility](https://tailwindcss.com/docs/compatibility) ·
[lucide installation](https://lucide.dev/guide/installation) ·
[lucide-svelte](https://lucide.dev/guide/packages/lucide-svelte) ·
[vite server-options](https://vite.dev/config/server-options) · registry.npmjs.org (live).

**Explicit uncertainty:** no `web_search` (API key missing) → no independent web corroboration. Versions are
live npm `latest` dist-tags queried in-session (authoritative for what `npm install` resolves today). Lucide
prop-level details and Tailwind's `@config` legacy path were not individually verified.
