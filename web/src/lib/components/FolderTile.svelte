<script lang="ts">
  import MiniIcon from '$lib/components/MiniIcon.svelte'
  import { board } from '$lib/store/board.svelte'
  import type { Item } from '$lib/types'

  interface Props {
    item: Item
    onopen: () => void
    onedit: () => void
    ondelete: () => void
  }
  let { item, onopen, onedit, ondelete }: Props = $props()

  const links = $derived(board.childLinks(item))
  const folder = $derived(board.folderOf(item))
  const label = $derived(folder?.name || `文件夹（${links.length} 个）`)
</script>

<div class="relative size-[var(--tile)]">
  <button
    type="button"
    onclick={onopen}
    class="size-full cursor-pointer rounded-[var(--radius-tile)] bg-fg/10 p-1.5 ring-1 ring-fg/15 backdrop-blur-sm transition
           hover:bg-fg/20 hover:ring-fg/30 focus-visible:ring-3 focus-visible:ring-accent-500 focus-visible:outline-none"
    aria-label="打开 {label}"
    aria-haspopup="dialog"
    title={label}
  >
    <!-- 小夹：固定 3x3 = 9 个缩略位（Q15 决策 1） -->
    <span class="grid size-full grid-cols-3 grid-rows-3 gap-[2px] text-[0.5rem]">
      {#each Array(9) as _, i (i)}
        {#if links[i]}
          <MiniIcon link={links[i]} />
        {:else}
          <span class="aspect-square rounded-lg bg-fg/5"></span>
        {/if}
      {/each}
    </span>
  </button>

  <div
    class="absolute -top-1 -right-1 flex gap-1 opacity-0 transition group-hover:opacity-100 focus-within:opacity-100"
  >
    <button
      type="button"
      onclick={onedit}
      class="flex size-5 cursor-pointer items-center justify-center rounded-full bg-surface-700 text-[10px] text-fg/80 ring-1 ring-fg/20 hover:bg-accent-500 hover:text-white"
      aria-label="编辑 {label}"
    >
      &#9998;
    </button>
    <button
      type="button"
      onclick={ondelete}
      class="flex size-5 cursor-pointer items-center justify-center rounded-full bg-surface-700 text-[10px] text-fg/80 ring-1 ring-fg/20 hover:bg-red-500 hover:text-white"
      aria-label="删除 {label}"
    >
      &#10005;
    </button>
  </div>
</div>
