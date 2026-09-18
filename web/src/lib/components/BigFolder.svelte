<script lang="ts">
  import MiniIcon from '$lib/components/MiniIcon.svelte'
  import { board } from '$lib/store/board.svelte'
  import type { Item } from '$lib/types'

  interface Props {
    item: Item
    onopen?: () => void
    onedit: () => void
    ondelete: () => void
  }
  let { item, onopen, onedit, ondelete }: Props = $props()

  const links = $derived(board.childLinks(item))
  const folder = $derived(board.folderOf(item))
  const label = $derived(folder?.name || `大文件夹（${links.length} 个）`)
</script>

<!--
  大文件夹（2x2 格）：内部 9 个图标**直接可点**（Q15 决策 3），
  而空白处（内边距 + 图标之间的缝）点开与小夹**同一个预览模态** ——
  在那里可以像普通图标一样逐个改夹内图标的图标。
  按钮垫在图标层**下面**（不是包在外面）：CSS 的 :hover 只沿祖先链传播，
  所以悬停某张图标时不会点亮"展开"的提示，只有真的指在空白处才亮。
-->
<div
  class="relative flex size-full flex-col rounded-[var(--radius-tile-lg)] bg-glass p-2 ring-1 ring-glass-ring frosted"
  aria-label={label}
>
  {#if onopen}
    <button
      type="button"
      onclick={onopen}
      class="group/open absolute inset-0 cursor-pointer rounded-[var(--radius-tile-lg)] focus-visible:ring-2 focus-visible:ring-accent-500 focus-visible:outline-none"
      aria-label="展开 {label}"
      aria-haspopup="dialog"
      title="展开预览"
      data-testid="bigfolder-open"
    >
      <span
        class="pointer-events-none absolute inset-0 rounded-[var(--radius-tile-lg)] ring-2 ring-transparent transition
               group-hover/open:bg-fg/10 group-hover/open:ring-accent-500"
      ></span>
    </button>
  {/if}

  <span class="relative z-10 grid flex-1 grid-cols-3 grid-rows-3 gap-1 text-[0.55rem]">
    {#each Array(9) as _, i (i)}
      {#if links[i]}
        <MiniIcon link={links[i]} mode="live" />
      {:else}
        <span class="aspect-square rounded-lg bg-fg/5"></span>
      {/if}
    {/each}
  </span>

  <div
    class="absolute -top-1 -right-1 z-20 flex gap-1 opacity-0 transition group-hover:opacity-100 focus-within:opacity-100"
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
