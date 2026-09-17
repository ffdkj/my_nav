<script lang="ts">
  import { board } from '$lib/store/board.svelte'
  import type { Item } from '$lib/types'

  export interface MenuTarget {
    item: Item
    x: number
    y: number
  }

  interface Props {
    target: MenuTarget | null
    onclose: () => void
    onedit: (item: Item) => void
    ondelete: (item: Item) => void
  }
  let { target, onclose, onedit, ondelete }: Props = $props()

  const others = $derived(
    target ? board.pages.filter((p) => p.id !== board.page?.id) : [],
  )

  async function moveTo(pageId: string) {
    if (!target) return
    const item = target.item
    onclose()
    await board.moveItemToPage(item.id, pageId)
  }
</script>

{#if target}
  <div
    class="fixed inset-0 z-[60]"
    role="presentation"
    onclick={onclose}
    oncontextmenu={(e) => {
      e.preventDefault()
      onclose()
    }}
  ></div>
  <div
    class="fixed z-[61] min-w-44 overflow-hidden rounded-xl bg-surface-800 py-1 text-sm ring-1 ring-white/15"
    style="left: {Math.min(target.x, window.innerWidth - 190)}px; top: {Math.min(target.y, window.innerHeight - 220)}px;"
    role="menu"
    aria-label="图标操作"
  >
    <button
      type="button"
      onclick={() => {
        onedit(target.item)
        onclose()
      }}
      class="block w-full cursor-pointer px-4 py-2 text-left hover:bg-white/10"
      role="menuitem"
    >
      编辑
    </button>

    {#if target.item.kind === 'folder'}
      <button
        type="button"
        onclick={() => {
          const next = (target.item.size ?? 1) === 2 ? 1 : 2
          void board.setFolderSize(target.item.id, next as 1 | 2)
          onclose()
        }}
        class="block w-full cursor-pointer px-4 py-2 text-left hover:bg-white/10"
        role="menuitem"
      >
        {(target.item.size ?? 1) === 2 ? '缩小为 1 格' : '放大为 2×2'}
      </button>
    {/if}

    {#if others.length > 0}
      <div class="my-1 border-t border-white/10"></div>
      <p class="px-4 py-1 text-xs text-white/40">移动到…</p>
      {#each others as p (p.id)}
        <button
          type="button"
          onclick={() => moveTo(p.id)}
          class="block w-full cursor-pointer px-4 py-2 text-left hover:bg-white/10"
          role="menuitem"
        >
          {p.name}
        </button>
      {/each}
    {/if}

    <div class="my-1 border-t border-white/10"></div>
    <button
      type="button"
      onclick={() => {
        ondelete(target.item)
        onclose()
      }}
      class="block w-full cursor-pointer px-4 py-2 text-left text-red-300 hover:bg-red-500/20"
      role="menuitem"
    >
      删除
    </button>
  </div>
{/if}
