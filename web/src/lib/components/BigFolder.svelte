<script lang="ts">
  import MiniIcon from '$lib/components/MiniIcon.svelte'
  import { board } from '$lib/store/board.svelte'
  import type { Item } from '$lib/types'

  interface Props {
    item: Item
    onedit: () => void
    ondelete: () => void
  }
  let { item, onedit, ondelete }: Props = $props()

  const links = $derived(board.childLinks(item))
  const folder = $derived(board.folderOf(item))
  const label = $derived(folder?.name || `大文件夹（${links.length} 个）`)
</script>

<!--
  大文件夹（2x2 格）：内部 9 个图标**直接可点**，没有模态（Q15 决策 3）。
  管理（拖拽/删除）走 hover 编辑按钮 —— 桌面端；触屏的长按菜单在 M4 与「移动到…」一起做。
-->
<div
  class="relative flex size-full flex-col rounded-[var(--radius-tile)] bg-fg/[0.07] p-2 ring-1 ring-fg/15 backdrop-blur-sm"
  aria-label={label}
>
  <span class="grid flex-1 grid-cols-3 grid-rows-3 gap-1 text-[0.55rem]">
    {#each Array(9) as _, i (i)}
      {#if links[i]}
        <MiniIcon link={links[i]} mode="live" />
      {:else}
        <span class="aspect-square rounded-lg bg-fg/5"></span>
      {/if}
    {/each}
  </span>

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
