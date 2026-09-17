<script lang="ts">
  import { Moon, Sun } from '@lucide/svelte'
  import FolderDialog from '$lib/components/FolderDialog.svelte'
  import FolderModal from '$lib/components/FolderModal.svelte'
  import Grid from '$lib/components/Grid.svelte'
  import LinkDialog from '$lib/components/LinkDialog.svelte'
  import SearchBar from '$lib/components/SearchBar.svelte'
  import Toasts from '$lib/components/Toasts.svelte'
  import { api } from '$lib/api'
  import { board } from '$lib/store/board.svelte'
  import { ui } from '$lib/store/ui.svelte'
  import type { Item, Link } from '$lib/types'

  let dark = $state(true)
  let dialogOpen = $state(false)
  let editing = $state<Link | null>(null)
  let openFolder = $state<Item | null>(null)
  let editFolder = $state<Item | null>(null)

  let started = false
  $effect(() => {
    if (!started) {
      started = true
      void board.boot()
    }
  })

  $effect(() => {
    document.documentElement.classList.toggle('dark', dark)
  })

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
</script>

<main class="relative min-h-dvh">
  <!-- 壁纸层（M6 接入真实壁纸，这里先给一个可用的渐变兜底） -->
  <div
    class="pointer-events-none fixed inset-0 -z-10 bg-[radial-gradient(circle_at_20%_10%,#16203a,transparent_55%),radial-gradient(circle_at_80%_0%,#0d2a4a,transparent_45%)]"
    aria-hidden="true"
  ></div>

  <div class="mx-auto flex max-w-6xl flex-col gap-6 px-4 py-8">
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
    </header>

    {#if board.page}
      <p class="text-xs text-white/40">
        {board.page.name}
        · {board.sequence.length} 个图标
        {#if board.status === 'saving'}· 保存中…{/if}
        {#if board.status === 'loading'}· 加载中…{/if}
      </p>
    {/if}

    {#if board.status === 'loading' && board.sequence.length === 0}
      <p class="py-20 text-center text-sm text-white/50">加载中…</p>
    {:else}
      <Grid
        onadd={openAdd}
        onedit={openEdit}
        ondelete={confirmDelete}
        onopenfolder={(item) => (openFolder = item)}
      />
    {/if}

    <footer class="pt-4 text-xs text-white/30">
      {#if board.lastError}
        上次操作失败：{board.lastError}
      {:else}
        M3 · 拖拽吸附 / 悬停 {board.mergeDwellMs}ms 合并为文件夹 / 小夹模态 / 大夹 2×2 直接可点
      {/if}
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

<Toasts />
