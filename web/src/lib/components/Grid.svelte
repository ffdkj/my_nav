<script lang="ts">
  import { untrack } from 'svelte'
  import { dndzone, TRIGGERS, SHADOW_PLACEHOLDER_ITEM_ID, type DndEvent } from 'svelte-dnd-action'
  import Tile from '$lib/components/Tile.svelte'
  import { pack, rowCount } from '$lib/layout'
  import { board, GRID_COLS } from '$lib/store/board.svelte'
  import type { Item } from '$lib/types'

  interface Props {
    onadd: () => void
    onedit: (item: Item) => void
    ondelete: (item: Item) => void
    onopenfolder: (item: Item) => void
  }
  let { onadd, onedit, ondelete, onopenfolder }: Props = $props()

  let container = $state<HTMLElement | null>(null)
  let probe = $state<HTMLElement | null>(null)
  let displayCols = $state(GRID_COLS)
  let tilePx = $state(96)
  let gapPx = $state(16)

  const spanOf = (item: Item) => (item.kind === 'folder' ? (item.size ?? 1) : 1)

  // 两个必须遵守的约束（都是实测踩出来的，详见 e2e/README.md）：
  //  1) 传给 dndzone 的数组里每个元素必须**顶层带 id**，位置信息只能旁挂。
  //  2) 承载 dndzone 的元素必须**有真实布局盒**（不能用 display: contents）。
  let zoneItems = $state<Item[]>(untrack(() => [...board.sequence]))
  $effect(() => {
    zoneItems = [...board.sequence]
  })

  const ADD_ID = '__add__'
  const addCell: Item = { id: ADD_ID, kind: 'link', children: [] }

  const packedAll = $derived(pack([...zoneItems, addCell], displayCols, spanOf))
  const placement = $derived(
    new Map(packedAll.filter((p) => p.item.id !== ADD_ID).map((p) => [p.item.id, p])),
  )
  const addSlot = $derived(packedAll.find((p) => p.item.id === ADD_ID))
  const rows = $derived(Math.max(1, rowCount(packedAll)))
  const gridHeight = $derived(rows * (tilePx + gapPx) - gapPx)

  // ---------- 合并（悬停 dwell）----------
  // 库只给 {trigger, id, source}，没有任何"悬停在哪个图块上"的信息，
  // 所以命中判定由我们自己用**与渲染完全相同的网格数学**算出来。
  let draggedId: string | null = $state(null)
  let mergeArmedFor: string | null = $state(null)
  let hoverTarget: string | null = null
  let dwellTimer: ReturnType<typeof setTimeout> | undefined

  function clearDwell() {
    if (dwellTimer) clearTimeout(dwellTimer)
    dwellTimer = undefined
  }

  function resetMerge() {
    clearDwell()
    hoverTarget = null
    mergeArmedFor = null
  }

  // ⚠️ 命中判定必须用**拖拽开始那一刻**的布局快照。
  // 库在拖拽过程中会实时重排：把 A 拖到 B 上时，A 的 shadow 占位就落在 B 的格子里，
  // 于是"指针当前所在的格子"永远指向自己，永远判定不出合并目标。
  // 锁定起始布局后，"指针压在 B 原来的格子上" 就是用户心智里的"我悬停在 B 上"。
  let hitCells: Array<{ id: string; col: number; row: number; span: number }> = []

  function captureHitCells() {
    hitCells = packedAll
      .filter((p) => p.item.id !== ADD_ID)
      .map((p) => ({ id: p.item.id, col: p.col, row: p.row, span: p.span }))
  }

  function tileAtPoint(clientX: number, clientY: number): string | null {
    const el = container
    if (!el) return null
    const rect = el.getBoundingClientRect()
    const col = Math.floor((clientX - rect.left) / (tilePx + gapPx))
    const row = Math.floor((clientY - rect.top) / (tilePx + gapPx))
    if (col < 0 || row < 0 || col >= displayCols) return null
    for (const cell of hitCells) {
      if (col >= cell.col && col < cell.col + cell.span && row >= cell.row && row < cell.row + cell.span) {
        return cell.id
      }
    }
    return null
  }

  function onPointerMove(ev: PointerEvent) {
    if (!draggedId) return
    const id = tileAtPoint(ev.clientX, ev.clientY)
    // 拖拽期间，被拖项在 zoneItems 里被替换成 shadow 占位（id 固定为
    // 'id:dnd-shadow-placeholder-0000'），它代表的就是"自己"，不能当成合并目标。
    const isSelf = !id || id === draggedId || id === SHADOW_PLACEHOLDER_ITEM_ID
    if (!isSelf) {
      if (id !== hoverTarget) {
        hoverTarget = id
        mergeArmedFor = null
        clearDwell()
        const candidate = id
        dwellTimer = setTimeout(() => {
          // 悬停达到阈值 → 武装合并，松手时执行（不在拖拽中途改结构，避免库的状态错乱）
          if (hoverTarget === candidate && draggedId) mergeArmedFor = candidate
        }, board.mergeDwellMs)
      }
    } else {
      resetMerge()
    }
  }

  function attachTracking() {
    window.addEventListener('pointermove', onPointerMove, { passive: true })
  }

  function detachTracking() {
    window.removeEventListener('pointermove', onPointerMove)
  }

  // 显示列数 = floor((可用宽 + gap) / (tile + gap))，上限 12 列（与服务端一致）
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
    return () => {
      ro.disconnect()
      detachTracking()
    }
  })

  function consider(e: CustomEvent<DndEvent<Item>>) {
    zoneItems = e.detail.items
    const trigger = e.detail.info.trigger
    if (trigger === TRIGGERS.DRAG_STARTED) {
      draggedId = e.detail.info.id
      resetMerge()
      captureHitCells()
      attachTracking()
    }
  }

  function finalize(e: CustomEvent<DndEvent<Item>>) {
    zoneItems = e.detail.items
    detachTracking()
    clearDwell()

    const source = draggedId
    const target = mergeArmedFor
    draggedId = null
    hoverTarget = null
    mergeArmedFor = null
    hitCells = []

    // 悬停达标后松手 = 合并（优先于排序）
    if (source && target && source !== target) {
      void board.mergeItems(source, target)
      return
    }

    // 正常情况下库会把 shadow 换回真实项；若没换回来说明状态异常，直接回读服务端
    const ids = zoneItems.map((i) => i.id).filter((id) => id !== SHADOW_PLACEHOLDER_ITEM_ID)
    if (ids.length !== board.sequence.length) {
      void board.reload()
      return
    }
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
          <Tile
            {item}
            mergeArmed={mergeArmedFor === item.id}
            onopen={() => onopenfolder(item)}
            onedit={() => onedit(item)}
            ondelete={() => ondelete(item)}
          />
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
