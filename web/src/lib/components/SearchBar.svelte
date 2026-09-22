<script lang="ts">
  import Fuse from 'fuse.js'
  import { Search } from '@lucide/svelte'
  import { api } from '$lib/api'
  import { board } from '$lib/store/board.svelte'
  import { ui } from '$lib/store/ui.svelte'
  import type { LinkWithPage } from '$lib/types'

  let query = $state('')
  let inputEl = $state<HTMLInputElement | null>(null)
  let allLinks = $state<LinkWithPage[]>([])
  let loaded = $state(false)
  let highlighted = $state(0)
  let pickerOpen = $state(false)
  let focused = $state(false)

  // 站内结果上限：与 Ctrl/Cmd+1…9 的九个槽位一一对应
  const MAX_RESULTS = 9

  // 全局搜索：一次性拉全量链接（个人规模，几百条以内），之后靠 setCollection 增量更新
  const fuse = new Fuse<LinkWithPage>([], {
    keys: [
      { name: 'title', weight: 3 },
      { name: 'url', weight: 2 },
      { name: 'page_name', weight: 1 },
    ],
    // 关键：默认 ignoreLocation=false 会把长 URL 深处的命中重罚
    ignoreLocation: true,
    threshold: 0.35,
    minMatchCharLength: 1,
  })

  async function ensureLoaded() {
    if (loaded) return
    try {
      const data = await api.get<{ links: LinkWithPage[] }>('/api/links')
      allLinks = data.links
      fuse.setCollection(allLinks)
      loaded = true
    } catch (err) {
      ui.error('加载链接索引失败：' + (err instanceof Error ? err.message : String(err)))
    }
  }

  const results = $derived.by(() => {
    if (!query.trim()) return []
    if (!loaded) return []
    return fuse.search(query.trim(), { limit: MAX_RESULTS })
  })

  const engine = $derived(board.defaultEngine)

  // 查询变了就把高亮拉回第一条。⚠️ 必须显式读一下 query：$effect 只追踪"读过的"依赖，
  // 原先那句 `$effect(() => { highlighted = 0 })` 什么都没读，实际只在挂载时跑过一次 ——
  // 于是"上次高亮停在第 5 条，新查询只剩 3 条"时 Ctrl/Cmd+Enter 会静默失灵。
  $effect(() => {
    void query
    highlighted = 0
  })

  async function chooseEngine(id: string) {
    board.setSetting('default_engine_id', id)
    pickerOpen = false
    try {
      await api.patch('/api/settings', { default_engine_id: id })
    } catch {
      ui.error('默认搜索引擎保存失败')
    }
  }

  function openResult(link: LinkWithPage) {
    window.open(link.url, '_blank', 'noopener')
    query = ''
  }

  function runSearch() {
    if (!engine || !query.trim()) return
    const url = engine.url_tpl.replace('{query}', encodeURIComponent(query.trim()))
    window.open(url, '_blank', 'noopener')
    query = ''
  }

  // Q16 决策：回车永远走搜索引擎；Ctrl/Cmd+Enter 打开高亮的站内结果
  function onkeydown(e: KeyboardEvent) {
    // Ctrl/Cmd + 1…9：直接打开第 N 条站内匹配。
    //
    // 为什么必须 preventDefault：桌面浏览器把 Ctrl+数字 当"切换标签页"的快捷键，
    // 不抢这一下，你按下 Ctrl+1 就会被切到第 1 个标签页。**抢不抢得赢由浏览器决定**
    // （这个键会不会先发给网页，各浏览器不一样）；装成 PWA 时没有标签页，冲突根本
    // 不存在，必定生效。
    //
    // 判据用 e.code 而不是 e.key：按住 Ctrl 时某些布局/输入法下 e.key 并不是数字。
    // 小键盘也算（Numpad1）。越界（只有 3 条却按 Ctrl+5）**不抢键**：既然不处理，
    // 就别把浏览器本来的行为一并吃掉。
    if ((e.ctrlKey || e.metaKey) && !e.altKey && !e.shiftKey) {
      const digit = /^(?:Digit|Numpad)([1-9])$/.exec(e.code)
      const hit = digit ? results[Number(digit[1]) - 1] : undefined
      if (hit) {
        e.preventDefault()
        openResult(hit.item)
        return
      }
    }
    if (e.key === 'ArrowDown' && results.length) {
      e.preventDefault()
      highlighted = Math.min(highlighted + 1, results.length - 1)
      return
    }
    if (e.key === 'ArrowUp' && results.length) {
      e.preventDefault()
      highlighted = Math.max(highlighted - 1, 0)
      return
    }
    if (e.key === 'Escape') {
      query = ''
      inputEl?.blur()
      return
    }
    if (e.key === 'Enter') {
      e.preventDefault()
      if ((e.ctrlKey || e.metaKey) && results[highlighted]) {
        openResult(results[highlighted].item)
        return
      }
      runSearch()
    }
  }

  // "/" 或 Ctrl/Cmd+K 聚焦搜索框（在 App 层也会用到）
  export function focus() {
    inputEl?.focus()
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if (e.key === '/' && document.activeElement !== inputEl) {
      e.preventDefault()
      inputEl?.focus()
    }
  }}
/>

<div class="relative w-full max-w-2xl">
  <div
    class="flex items-center gap-2 rounded-full bg-glass px-3 py-2 ring-1 ring-glass-ring focus-within:ring-2 focus-within:ring-accent-500"
  >
    <button
      type="button"
      onclick={() => (pickerOpen = !pickerOpen)}
      class="flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-full text-xs font-semibold text-white"
      style="background:{engine?.icon_color ?? 'var(--color-accent-500)'}"
      aria-label="切换搜索引擎（当前：{engine?.name ?? '未设置'}）"
      aria-expanded={pickerOpen}
    >
      {engine?.icon_text ?? '?'}
    </button>

    <input
      bind:this={inputEl}
      bind:value={query}
      onkeydown={onkeydown}
      onfocus={() => {
        focused = true
        void ensureLoaded()
      }}
      onblur={() => setTimeout(() => (focused = false), 150)}
      placeholder="输入并搜索"
      class="w-full bg-transparent text-sm outline-none placeholder:text-fg/50"
      aria-label="搜索"
      autocomplete="off"
    />

    <Search size={16} class="shrink-0 opacity-60" />
  </div>

  {#if pickerOpen}
    <div
      class="absolute top-full left-0 z-40 mt-2 w-64 rounded-xl bg-surface-800 p-1 ring-1 ring-fg/15"
      role="listbox"
      aria-label="搜索引擎"
    >
      {#each board.engines as eng (eng.id)}
        <button
          type="button"
          onclick={() => chooseEngine(eng.id)}
          class="flex w-full cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-left text-sm hover:bg-fg/10
                 {eng.id === engine?.id ? 'text-accent-500' : 'text-fg/85'}"
          role="option"
          aria-selected={eng.id === engine?.id}
        >
          <span
            class="flex size-5 items-center justify-center rounded-full text-[10px] font-semibold text-white"
            style="background:{eng.icon_color}"
          >
            {eng.icon_text}
          </span>
          {eng.name}
        </button>
      {/each}
    </div>
  {/if}

  {#if focused && query.trim()}
    <div
      class="absolute top-full left-0 z-30 mt-2 w-full overflow-hidden rounded-xl bg-surface-800 ring-1 ring-fg/15"
    >
      {#if results.length === 0}
        <p class="px-4 py-3 text-sm text-fg/50">站内没有匹配，回车用 {engine?.name} 搜索</p>
      {:else}
        <!-- 9 条在手机上会顶到键盘，所以列表自己滚，底部提示行固定可见 -->
        <ul class="max-h-[50vh] overflow-y-auto">
          {#each results as r, i (r.item.id)}
            <li>
              <button
                type="button"
                data-testid="search-result"
                data-link-id={r.item.id}
                onmousedown={(e) => {
                  e.preventDefault()
                  openResult(r.item)
                }}
                class="flex w-full cursor-pointer items-center gap-3 px-4 py-2 text-left text-sm
                       {i === highlighted ? 'bg-fg/10' : ''}"
              >
                <span
                  class="flex size-5 shrink-0 items-center justify-center rounded-md bg-fg/10 text-[11px] text-fg/60 tabular-nums"
                  data-testid="search-result-index">{i + 1}</span
                >
                <span class="min-w-0 flex-1 truncate">{r.item.title}</span>
                <span class="shrink-0 text-xs text-fg/40">{r.item.page_name}</span>
              </button>
            </li>
          {/each}
        </ul>
        <p class="border-t border-fg/10 px-4 py-2 text-xs text-fg/40">
          回车 = 用 {engine?.name} 搜索 · Ctrl/Cmd+1…9 = 打开第 N 条 · Ctrl/Cmd+Enter = 打开选中项
        </p>
      {/if}
    </div>
  {/if}
</div>
