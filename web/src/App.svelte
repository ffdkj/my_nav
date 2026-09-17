<script lang="ts">
  import { Moon, Plus, Search, Sun } from '@lucide/svelte'
  import Tile from '$lib/components/Tile.svelte'
  import { board } from '$lib/store/board.svelte'
  import type { Link } from '$lib/types'

  // M0 骨架：用本地 mock 数据验证工具链（Svelte 5 runes + Tailwind v4 + Lucide + 拖拽库）。
  // M1 起改为 GET /api/pages/{id}/board。

  let dark = $state(true)
  let query = $state('')

  const demo: Link[] = [
    mk('1', 'GitHub', 'https://github.com'),
    mk('2', 'DeepSeek', 'https://chat.deepseek.com'),
    mk('3', 'PostHog', 'https://us.posthog.com'),
    mk('4', 'Cloudflare', 'https://dash.cloudflare.com'),
    mk('5', 'Zeabur', 'https://zeabur.com'),
    mk('6', 'Gitee', 'https://gitee.com'),
  ]

  function mk(id: string, title: string, url: string): Link {
    return {
      id,
      title,
      url,
      open_new_tab: true,
      icon_source: 'auto',
      icon_path: null,
      icon_status: 'miss',
      mono_text: null,
      mono_color: 'var(--color-accent-500)',
      mono_font_size: 30,
    }
  }

  $effect(() => {
    document.documentElement.classList.toggle('dark', dark)
  })

  board.apply({
    page: {
      id: 'page-home',
      slug: 'home',
      name: '主页',
      sort_order: 0,
      wallpaper_mode: 'global',
      wallpaper_id: null,
    },
    revision: 1,
    items: demo.map((l, i) => ({ id: `pl-${l.id}`, kind: 'link' as const, link_id: l.id, col: i % 6, row: 0 })),
    links: demo,
    folders: [],
  })
</script>

<main class="relative min-h-dvh">
  <div class="mx-auto flex max-w-5xl flex-col gap-8 px-4 py-10">
    <header class="flex items-center justify-between">
      <h1 class="text-lg font-semibold tracking-tight">my_nav</h1>
      <button
        type="button"
        class="rounded-full bg-white/10 p-2 ring-1 ring-white/15 hover:bg-white/20"
        aria-label={dark ? '切换到浅色主题' : '切换到深色主题'}
        onclick={() => (dark = !dark)}
      >
        {#if dark}<Moon size={18} />{:else}<Sun size={18} />{/if}
      </button>
    </header>

    <div class="flex items-center gap-2 rounded-full bg-white/10 px-4 py-3 ring-1 ring-white/15">
      <Search size={18} class="shrink-0 opacity-70" />
      <input
        bind:value={query}
        placeholder="输入并搜索"
        class="w-full bg-transparent text-sm outline-none placeholder:text-white/50"
        aria-label="搜索"
      />
    </div>

    <nav aria-label="应用图块">
      <ul class="flex flex-wrap gap-[var(--gap)]">
        {#each demo as link (link.id)}
          <Tile {link} />
        {/each}
        <li class="flex w-[var(--tile)] flex-col items-center gap-2">
          <button
            type="button"
            class="flex size-[var(--tile)] items-center justify-center rounded-[var(--radius-tile)]
                   border border-dashed border-white/25 text-white/50 hover:bg-white/5"
            aria-label="添加图标"
          >
            <Plus size={20} />
          </button>
          <span class="text-xs text-white/50">添加</span>
        </li>
      </ul>
    </nav>

    <footer class="text-xs text-white/40">
      M0 骨架 · 工具链自检（Svelte 5 / Vite 8 / Tailwind v4 / Lucide）
    </footer>
  </div>
</main>
