<script lang="ts">
  import MiniIcon from '$lib/components/MiniIcon.svelte'
  import { pack } from '$lib/layout'
  import { board } from '$lib/store/board.svelte'

  // 跨页拖拽的漂浮层：拖到屏幕边缘翻页后，由它接管指针跟随，
  // 松手时把落点下标交给 store，走 /api/board/move 原子落库。
  let pos = $state({ x: 0, y: 0 })
  let index = $state(0)

  const carried = $derived(board.carry)
  const link = $derived(carried?.item.link_id ? board.links[carried.item.link_id] : undefined)

  function indexAt(x: number, y: number): number {
    const m = board.gridMetrics
    const col = Math.floor((x - m.left) / (m.tile + m.gap))
    const row = Math.floor((y - m.top) / (m.tile + m.gap))
    const placed = pack(board.sequence, m.cols, (i) =>
      i.kind === 'folder' ? (i.size ?? 1) : 1,
    )
    const found = placed.findIndex((p) => p.row > row || (p.row === row && p.col >= col))
    return found < 0 ? placed.length : found
  }

  $effect(() => {
    if (!carried) return
    const onMove = (e: PointerEvent) => {
      pos = { x: e.clientX, y: e.clientY }
      index = indexAt(e.clientX, e.clientY)
    }
    const onUp = () => {
      void board.finishCarry(index)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') board.cancelCarry()
    }
    window.addEventListener('pointermove', onMove, { passive: true })
    window.addEventListener('pointerup', onUp)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('keydown', onKey)
    }
  })
</script>

{#if carried}
  <div
    class="pointer-events-none fixed z-[70] flex size-[var(--tile)] items-center justify-center rounded-[var(--radius-tile)] bg-glass-hover opacity-90 ring-2 ring-accent-500 frosted"
    style="left: {pos.x - 32}px; top: {pos.y - 32}px;"
    aria-hidden="true"
  >
    {#if link}
      <MiniIcon {link} />
    {:else}
      <span class="text-2xl">&#128193;</span>
    {/if}
  </div>
  <div
    class="pointer-events-none fixed z-[70] -translate-x-1/2 rounded-full bg-surface-800/90 px-3 py-1 text-xs text-fg ring-1 ring-fg/20"
    style="left: {pos.x}px; top: {pos.y + 44}px;"
  >
    松手放到「{board.page?.name ?? ''}」
  </div>
{/if}
