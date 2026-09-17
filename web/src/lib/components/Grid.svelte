<script lang="ts">
  import { untrack } from 'svelte'
  import { dndzone, type DndEvent } from 'svelte-dnd-action'
  import Tile from '$lib/components/Tile.svelte'
  import { pack, rowCount } from '$lib/layout'
  import { board, GRID_COLS } from '$lib/store/board.svelte'
  import type { Item } from '$lib/types'

  interface Props {
    onadd: () => void
    onedit: (item: Item) => void
    ondelete: (item: Item) => void
  }
  let { onadd, onedit, ondelete }: Props = $props()

  let container = $state<HTMLElement | null>(null)
  let probe = $state<HTMLElement | null>(null)
  let displayCols = $state(GRID_COLS)
  let tilePx = $state(96)
  let gapPx = $state(16)

  const spanOf = (item: Item) => (item.kind === 'folder' ? (item.size ?? 1) : 1)

  // 两个必须遵守的约束（都是实测踩出来的）：
  //  1) 传给 dndzone 的数组里每个元素必须**顶层带 id**，位置信息只能旁挂，
  //     否则库抛 "missing 'id' property for item"，表现为完全拖不动。
  //  2) 承载 dndzone 的元素必须**有真实布局盒**。早先为了把「+」并进同一个网格
  //     而用了 display:contents，导致 zone 的 rect 恒为 0x0，落点判定失效
  //     （库调试输出："element was dropped right after it left origin but before
  //     entering somewhere else"），拖拽看起来"没反应"。
  //     所以这里 ul 是真实网格，「+」按打包结果绝对定位。
  let zoneItems = $state<Item[]>(untrack(() => [...board.sequence]))
  $effect(() => {
    zoneItems = [...board.sequence]
  })

  const ADD_ID = '__add__'
  const addCell: Item = { id: ADD_ID, kind: 'link', children: [] }

  // 一次打包同时得到：每个图块的位置 + 「+」应该落在哪一格
  const packedAll = $derived(pack([...zoneItems, addCell], displayCols, spanOf))
  const placement = $derived(
    new Map(packedAll.filter((p) => p.item.id !== ADD_ID).map((p) => [p.item.id, p])),
  )
  const addSlot = $derived(packedAll.find((p) => p.item.id === ADD_ID))
  const rows = $derived(Math.max(1, rowCount(packedAll)))
  const gridHeight = $derived(rows * (tilePx + gapPx) - gapPx)

  // 显示列数 = floor((可用宽 + gap) / (tile + gap))，上限 12 列（与服务端一致）。
  // 用 width:var(--tile) 的探针量出真实像素尺寸，响应式只写在 CSS 里。
  $effect(() => {
    const el = container
    const probeEl = probe
    if (!el || !probeEl) return
    const measure = () => {
      tilePx = probeEl.offsetWidth || 96
      gapPx = parseFloat(getComputedStyle(el).columnGap || '16') || 16
      const width = el.clientWidth || tilePx
      displayCols = Math.max(1, Math.min(GRID_COLS, Math.floor((width + gapPx) / (tilePx + gapPx))))
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  })

  function consider(e: CustomEvent<DndEvent<Item>>) {
    zoneItems = e.detail.items
  }

  function finalize(e: CustomEvent<DndEvent<Item>>) {
    zoneItems = e.detail.items
    const ids = zoneItems.map((i) => i.id)
    const current = board.sequence.map((i) => i.id)
    if (ids.join('\u0000') !== current.join('\u0000')) {
      void board.reorder(ids)
    }
  }
</script>

<div bind:this={container} class="relative w-full" data-cols={displayCols} data-rows={rows}>
  <span
    bind:this={probe}
    class="pointer-events-none absolute h-0 w-[var(--tile)]"
    aria-hidden="true"
  ></span>

  <ul
    use:dndzone={{
      items: zoneItems,
      type: 'tile',
      flipDurationMs: 200,
      delayTouchStart: 300,
      useCursorForDetection: true,
      dropAnimationDisabled: false,
    }}
    onconsider={consider}
    onfinalize={finalize}
    class="relative m-0 grid list-none justify-start p-0"
    style="grid-template-columns: repeat({displayCols}, var(--tile)); gap: {gapPx}px; min-height: {gridHeight}px;"
    aria-label="导航图标"
  >
    {#each zoneItems as item (item.id)}
      {@const cell = placement.get(item.id)}
      {#if cell}
        <li
          class="group relative"
          style="grid-column: {cell.col + 1} / span {cell.span}; grid-row: {cell.row + 1} / span {cell.span};"
        >
          <Tile {item} onedit={() => onedit(item)} ondelete={() => ondelete(item)} />
        </li>
      {/if}
    {/each}
  </ul>

  {#if addSlot}
    <button
      type="button"
      onclick={onadd}
      style="left: {addSlot.col * (tilePx + gapPx)}px; top: {addSlot.row * (tilePx + gapPx)}px; width: {tilePx}px; height: {tilePx}px;"
      class="absolute flex cursor-pointer flex-col items-center justify-center gap-1 rounded-[var(--radius-tile)]
             border border-dashed border-white/25 text-white/50 transition
             hover:border-accent-500 hover:text-accent-500 focus-visible:ring-3 focus-visible:ring-accent-500 focus-visible:outline-none"
      aria-label="添加图标"
    >
      <span class="text-2xl leading-none">+</span>
      <span class="text-xs">添加</span>
    </button>
  {/if}
</div>
