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
    return fuse.search(query.trim(), { limit: 8 })
  })

  const engine = $derived(board.defaultEngine)

  $effect(() => {
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
    class="flex items-center gap-2 rounded-full bg-white/10 px-3 py-2 ring-1 ring-white/15 focus-within:ring-2 focus-within:ring-accent-500"
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
      class="w-full bg-transparent text-sm outline-none placeholder:text-white/50"
      aria-label="搜索"
      autocomplete="off"
    />

    <Search size={16} class="shrink-0 opacity-60" />
  </div>

  {#if pickerOpen}
    <div
      class="absolute top-full left-0 z-40 mt-2 w-64 rounded-xl bg-surface-800 p-1 ring-1 ring-white/15"
      role="listbox"
      aria-label="搜索引擎"
    >
      {#each board.engines as eng (eng.id)}
        <button
          type="button"
          onclick={() => chooseEngine(eng.id)}
          class="flex w-full cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-left text-sm hover:bg-white/10
                 {eng.id === engine?.id ? 'text-accent-500' : 'text-white/85'}"
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
      class="absolute top-full left-0 z-30 mt-2 w-full overflow-hidden rounded-xl bg-surface-800 ring-1 ring-white/15"
    >
      {#if results.length === 0}
        <p class="px-4 py-3 text-sm text-white/50">站内没有匹配，回车用 {engine?.name} 搜索</p>
      {:else}
        <ul>
          {#each results as r, i (r.item.id)}
            <li>
              <button
                type="button"
                onmousedown={(e) => {
                  e.preventDefault()
                  openResult(r.item)
                }}
                class="flex w-full cursor-pointer items-center gap-3 px-4 py-2 text-left text-sm
                       {i === highlighted ? 'bg-white/10' : ''}"
              >
                <span class="min-w-0 flex-1 truncate">{r.item.title}</span>
                <span class="shrink-0 text-xs text-white/40">{r.item.page_name}</span>
              </button>
            </li>
          {/each}
        </ul>
        <p class="border-t border-white/10 px-4 py-2 text-xs text-white/40">
          回车 = 用 {engine?.name} 搜索 · Ctrl/Cmd+Enter = 打开选中项
        </p>
      {/if}
    </div>
  {/if}
</div>
