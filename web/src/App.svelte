<script lang="ts">
  import { Moon, Settings, Sun } from '@lucide/svelte'
  import CarryLayer from '$lib/components/CarryLayer.svelte'
  import ContextMenu, { type MenuTarget } from '$lib/components/ContextMenu.svelte'
  import FolderDialog from '$lib/components/FolderDialog.svelte'
  import FolderModal from '$lib/components/FolderModal.svelte'
  import Grid from '$lib/components/Grid.svelte'
  import LinkDialog from '$lib/components/LinkDialog.svelte'
  import PageDots from '$lib/components/PageDots.svelte'
  import PageManager from '$lib/components/PageManager.svelte'
  import SearchBar from '$lib/components/SearchBar.svelte'
  import SettingsDialog from '$lib/components/SettingsDialog.svelte'
  import WallpaperLayer from '$lib/components/WallpaperLayer.svelte'
  import Toasts from '$lib/components/Toasts.svelte'
  import { api } from '$lib/api'
  import { board } from '$lib/store/board.svelte'
  import { pwa } from '$lib/store/pwa.svelte'
  import { ui } from '$lib/store/ui.svelte'
  import type { Item, Link } from '$lib/types'

  let dark = $state(true)
  let dialogOpen = $state(false)
  let editing = $state<Link | null>(null)
  let openFolder = $state<Item | null>(null)
  let editFolder = $state<Item | null>(null)
  let managerOpen = $state(false)
  let settingsOpen = $state(false)
  let menu = $state<MenuTarget | null>(null)

  let started = false
  $effect(() => {
    if (!started) {
      started = true
      void board.boot()
      pwa.init()
    }
  })

  $effect(() => {
    document.documentElement.classList.toggle('dark', dark)
  })

  // ---------- 翻页 ----------

  function flip(dir: -1 | 1): boolean {
    const p = board.adjacentPage(dir)
    if (!p) return false
    void board.selectPage(p.id)
    return true
  }

  // 滚轮：累积超过阈值翻一页，400ms 内不重复触发
  let wheelAcc = 0
  let wheelLockUntil = 0
  function onwheel(e: WheelEvent) {
    if (board.carry || board.pages.length < 2) return
    const now = Date.now()
    if (now < wheelLockUntil) return
    wheelAcc += e.deltaY
    if (Math.abs(wheelAcc) >= 120) {
      if (flip(wheelAcc > 0 ? 1 : -1)) {
        wheelAcc = 0
        wheelLockUntil = now + 400
      }
    }
  }

  // 手势横滑：只在非图块区域起手，横向位移 >60 且纵向 <40 才算翻页
  let swipeStart: { x: number; y: number } | null = null
  function onpointerdown(e: PointerEvent) {
    if ((e.target as HTMLElement | null)?.closest('[data-tile]')) return
    swipeStart = { x: e.clientX, y: e.clientY }
  }
  function onpointerup(e: PointerEvent) {
    if (!swipeStart || board.carry) {
      swipeStart = null
      return
    }
    const dx = e.clientX - swipeStart.x
    const dy = e.clientY - swipeStart.y
    swipeStart = null
    if (Math.abs(dx) > 60 && Math.abs(dy) < 40) flip(dx < 0 ? 1 : -1)
  }

  // 拖拽到屏幕左右边缘 → 翻页并进入跨页 carry
  function handleEdge(dir: -1 | 1, itemId: string) {
    const p = board.adjacentPage(dir)
    if (!p) {
      ui.info(dir === 1 ? '已经是最后一页' : '已经是第一页')
      return
    }
    board.beginCarry(itemId, p.id)
  }

  // ---------- 图标操作 ----------

  function openAdd() {
    editing = null
    dialogOpen = true
  }

  function openEdit(item: Item) {
    if (item.kind === 'folder') {
      editFolder = item
      return
    }
    editing = board.linkOf(item) ?? null
    dialogOpen = true
  }

  async function submitDialog(url: string, title: string) {
    dialogOpen = false
    if (editing) {
      const target = editing
      const id = target.id
      board.links[id] = { ...target, url, title: title || target.title }
      try {
        await api.patch(`/api/links/${id}`, { url, title: title || target.title })
      } catch (err) {
        ui.error('保存失败：' + (err instanceof Error ? err.message : String(err)))
        await board.reload()
      }
    } else {
      await board.addLink(url, title)
    }
  }

  function confirmDelete(item: Item) {
    if (item.kind === 'folder') {
      editFolder = item
      return
    }
    const link = board.linkOf(item)
    const name = link?.title ?? '该图标'
    if (!window.confirm(`确定删除「${name}」吗？`)) return
    void board.removeItem(item.id)
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      menu = null
      if (board.carry) board.cancelCarry()
    }
  }
</script>

<svelte:window onkeydown={onkeydown} />

<main
  class="relative min-h-dvh"
  onwheel={onwheel}
  onpointerdown={onpointerdown}
  onpointerup={onpointerup}
>
  <WallpaperLayer />

  <div class="mx-auto flex min-h-dvh max-w-6xl flex-col gap-6 px-4 py-8">
    <header class="flex items-center gap-3">
      <h1 class="shrink-0 text-lg font-semibold tracking-tight">my_nav</h1>
      <div class="flex flex-1 justify-center">
        <SearchBar />
      </div>
      <button
        type="button"
        class="shrink-0 cursor-pointer rounded-full bg-white/10 p-2 ring-1 ring-white/15 hover:bg-white/20"
        aria-label={dark ? '切换到浅色主题' : '切换到深色主题'}
        onclick={() => (dark = !dark)}
      >
        {#if dark}<Moon size={18} />{:else}<Sun size={18} />{/if}
      </button>
      <button
        type="button"
        class="shrink-0 cursor-pointer rounded-full bg-white/10 p-2 ring-1 ring-white/15 hover:bg-white/20"
        aria-label="打开设置"
        onclick={() => (settingsOpen = true)}
      >
        <Settings size={18} />
      </button>
    </header>

    {#if board.page}
      <p class="text-xs text-white/40">
        {board.page.name}
        · {board.sequence.length} 个图标
        {#if board.status === 'saving'}· 保存中…{/if}
        {#if board.status === 'loading'}· 加载中…{/if}
        {#if board.carry}· 跨页移动中，松手放下{/if}
      </p>
    {/if}

    <div class="flex-1">
      {#if board.status === 'loading' && board.sequence.length === 0}
        <p class="py-20 text-center text-sm text-white/50">加载中…</p>
      {:else}
        <Grid
          onadd={openAdd}
          onedit={openEdit}
          ondelete={confirmDelete}
          onopenfolder={(item) => (openFolder = item)}
          onmenu={(target) => (menu = target)}
          onedge={handleEdge}
        />
      {/if}
    </div>

    <footer class="flex flex-col items-center gap-3 pt-4">
      {#if pwa.needRefresh}
        <div
          class="flex items-center gap-3 rounded-full bg-accent-500/95 px-4 py-1.5 text-xs text-white ring-1 ring-white/25"
          role="status"
        >
          有新版本可用
          <button
            type="button"
            onclick={() => pwa.applyUpdate()}
            class="cursor-pointer rounded-full bg-white/20 px-2 py-0.5 font-medium hover:bg-white/30"
          >
            刷新
          </button>
          <button
            type="button"
            onclick={() => pwa.dismiss()}
            class="cursor-pointer rounded-full px-2 py-0.5 hover:bg-white/20"
            aria-label="稍后再说"
          >
            稍后
          </button>
        </div>
      {/if}
      <PageDots onmanage={() => (managerOpen = true)} />
      <p class="text-xs text-white/30">
        {#if board.lastError}
          上次操作失败：{board.lastError}
        {:else}
          M6 · 壁纸（上传/外链/轮换）· 设置中心 · 引擎管理
        {/if}
      </p>
    </footer>
  </div>
</main>

<LinkDialog
  open={dialogOpen}
  link={editing}
  onsubmit={submitDialog}
  onclose={() => (dialogOpen = false)}
/>

<FolderModal item={openFolder} onclose={() => (openFolder = null)} />
<FolderDialog item={editFolder} onclose={() => (editFolder = null)} />
<PageManager open={managerOpen} onclose={() => (managerOpen = false)} />
<SettingsDialog open={settingsOpen} onclose={() => (settingsOpen = false)} />
<ContextMenu
  target={menu}
  onclose={() => (menu = null)}
  onedit={openEdit}
  ondelete={(item) => {
    menu = null
    confirmDelete(item)
  }}
/>
<CarryLayer />

<Toasts />
