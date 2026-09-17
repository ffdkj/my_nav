<script lang="ts">
  import { board } from '$lib/store/board.svelte'

  interface Props {
    onmanage: () => void
  }
  let { onmanage }: Props = $props()
</script>

<!-- 底部 iOS 式圆点（Q14 决策 1）。非编辑态只做切换与指示，管理入口在右侧 ⋯ -->
<nav class="flex items-center justify-center gap-2" aria-label="页面">
  {#each board.pages as p (p.id)}
    <button
      type="button"
      onclick={() => board.selectPage(p.id)}
      class="size-2.5 cursor-pointer rounded-full transition
             {p.id === board.page?.id ? 'scale-125 bg-white' : 'bg-white/30 hover:bg-white/60'}"
      aria-label="切换到 {p.name}"
      aria-current={p.id === board.page?.id}
      title={p.name}
    ></button>
  {/each}

  <button
    type="button"
    onclick={onmanage}
    class="ml-3 cursor-pointer rounded-full px-2 py-0.5 text-sm text-white/50 hover:bg-white/10 hover:text-white"
    aria-label="页面管理"
    title="页面管理"
  >
    &#8943;
  </button>
</nav>
