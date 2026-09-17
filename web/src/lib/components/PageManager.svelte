<script lang="ts">
  import { board } from '$lib/store/board.svelte'
  import { ui } from '$lib/store/ui.svelte'

  interface Props {
    open: boolean
    onclose: () => void
  }
  let { open, onclose }: Props = $props()

  let newName = $state('')
  let drafts = $state<Record<string, string>>({})

  $effect(() => {
    if (open) {
      drafts = Object.fromEntries(board.pages.map((p) => [p.id, p.name]))
      newName = ''
    }
  })

  async function add() {
    const name = newName.trim()
    if (!name) {
      ui.info('请输入页面名称')
      return
    }
    const page = await board.createPage(name)
    if (page) {
      newName = ''
      onclose()
    }
  }

  async function move(index: number, dir: -1 | 1) {
    const ids = board.pages.map((p) => p.id)
    const target = index + dir
    if (target < 0 || target >= ids.length) return
    ;[ids[index], ids[target]] = [ids[target], ids[index]]
    await board.reorderPages(ids)
  }
</script>

{#if open}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) onclose()
    }}
  >
    <div
      class="w-full max-w-md rounded-2xl bg-surface-800 p-6 ring-1 ring-fg/15"
      role="dialog"
      aria-modal="true"
      aria-label="页面管理"
    >
      <h2 class="mb-4 text-base font-semibold">页面管理</h2>

      <ul class="mb-4 flex flex-col gap-2">
        {#each board.pages as p, i (p.id)}
          <li class="flex items-center gap-2">
            <input
              value={drafts[p.id] ?? p.name}
              oninput={(e) => (drafts[p.id] = e.currentTarget.value)}
              onblur={() => {
                const next = (drafts[p.id] ?? '').trim()
                if (next && next !== p.name) void board.renamePage(p.id, next)
              }}
              class="min-w-0 flex-1 rounded-lg bg-fg/10 px-3 py-1.5 text-sm ring-1 ring-fg/15 outline-none focus:ring-2 focus:ring-accent-500"
              aria-label="页面名称"
            />
            <button
              type="button"
              onclick={() => move(i, -1)}
              disabled={i === 0}
              class="cursor-pointer rounded-lg px-2 py-1 text-xs text-fg/60 hover:bg-fg/10 disabled:opacity-30"
              aria-label="上移"
            >
              &#8593;
            </button>
            <button
              type="button"
              onclick={() => move(i, 1)}
              disabled={i === board.pages.length - 1}
              class="cursor-pointer rounded-lg px-2 py-1 text-xs text-fg/60 hover:bg-fg/10 disabled:opacity-30"
              aria-label="下移"
            >
              &#8595;
            </button>
            <button
              type="button"
              onclick={() => void board.deletePage(p.id)}
              disabled={board.pages.length <= 1}
              class="cursor-pointer rounded-lg px-2 py-1 text-xs text-red-600 dark:text-red-300 hover:bg-red-500/20 disabled:opacity-30"
              aria-label="删除页面 {p.name}"
            >
              删除
            </button>
          </li>
        {/each}
      </ul>

      <div class="mb-4 flex gap-2">
        <input
          bind:value={newName}
          placeholder="新页面名称"
          class="min-w-0 flex-1 rounded-lg bg-fg/10 px-3 py-2 text-sm ring-1 ring-fg/15 outline-none focus:ring-2 focus:ring-accent-500"
          aria-label="新页面名称"
          onkeydown={(e) => {
            if (e.key === 'Enter') void add()
          }}
        />
        <button
          type="button"
          onclick={add}
          class="cursor-pointer rounded-lg bg-accent-500 px-4 py-2 text-sm font-medium text-white hover:brightness-110"
        >
          新建
        </button>
      </div>

      <div class="flex justify-end">
        <button
          type="button"
          onclick={onclose}
          class="cursor-pointer rounded-lg bg-fg/10 px-4 py-2 text-sm hover:bg-fg/20"
        >
          完成
        </button>
      </div>
    </div>
  </div>
{/if}
