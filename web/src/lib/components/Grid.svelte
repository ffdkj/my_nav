<script lang="ts">
  import { untrack } from 'svelte'
  import { dndzone, TRIGGERS, SHADOW_PLACEHOLDER_ITEM_ID, type DndEvent } from 'svelte-dnd-action'
  import Tile from '$lib/components/Tile.svelte'
  import type { MenuTarget } from '$lib/components/ContextMenu.svelte'
  import { pack, rowCount } from '$lib/layout'
  import { board, GRID_COLS } from '$lib/store/board.svelte'
  import type { Item } from '$lib/types'

  interface Props {
    onadd: () => void
    onedit: (item: Item) => void
    ondelete: (item: Item) => void
    onopenfolder: (item: Item) => void
    onmenu: (target: MenuTarget) => void
    onedge: (dir: -1 | 1, itemId: string) => void
  }
  let { onadd, onedit, ondelete, onopenfolder, onmenu, onedge }: Props = $props()

  let container = $state<HTMLElement | null>(null)
  let probe = $state<HTMLElement | null>(null)
  let displayCols = $state(GRID_COLS)
  let tilePx = $state(96)
  let gapPx = $state(16)

  const spanOf = (item: Item) => (item.kind === 'folder' ? (item.size ?? 1) : 1)

  // 两个必须遵守的约束（都实测踩过，详见 e2e/README.md）：
  //  1) 传给 dndzone 的数组里每个元素必须顶层带 id，位置信息只能旁挂
  //  2) 承载 dndzone 的元素必须有真实布局盒（不能用 display: contents）
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
  let draggedId: string | null = $state(null)
  let mergeArmedFor: string | null = $state(null)
  let hoverTarget: string | null = null
  let dwellTimer: ReturnType<typeof setTimeout> | undefined

  // 命中判定用**拖拽开始那一刻**的布局快照：库会实时重排，
  // 拖 A 到 B 上时 A 的 shadow 就落在 B 的格子里，实时查表永远查到自己。
  let hitCells: Array<{ id: string; col: number; row: number; span: number }> = []

  function captureHitCells() {
    hitCells = packedAll
      .filter((p) => p.item.id !== ADD_ID)
      .map((p) => ({ id: p.item.id, col: p.col, row: p.row, span: p.span }))
  }

  // ---------- 屏幕边缘翻页 ----------
  const EDGE_PX = 60
  let edgeDir: -1 | 1 | null = null
  let edgeTimer: ReturnType<typeof setTimeout> | undefined

  function clearEdge() {
    if (edgeTimer) clearTimeout(edgeTimer)
    edgeTimer = undefined
    edgeDir = null
  }

  // ---------- 长按菜单 ----------
  const LONG_PRESS_MS = 800
  let pressTimer: ReturnType<typeof setTimeout> | undefined
  let pressStart: { x: number; y: number } | null = null

  function clearPress() {
    if (pressTimer) clearTimeout(pressTimer)
    pressTimer = undefined
    pressStart = null
    window.removeEventListener('pointermove', onPressMove)
  }

  // ⚠️ 取消长按必须监听 **window**，不能只绑在 <li> 上：
  // 拖拽期间库会把原元素隐藏、改用挂在 body 上的克隆，
  // 绑在 li 上的 pointermove 再也收不到事件，于是"一动就取消长按"失效，
  // 结果拖拽/跨页 carry 到 800ms 时会莫名弹出上下文菜单（已在 e2e 截图里抓到）。
  function onPressMove(e: PointerEvent) {
    if (!pressStart || !pressTimer) return
    if (Math.abs(e.clientX - pressStart.x) > 6 || Math.abs(e.clientY - pressStart.y) > 6) clearPress()
  }

  function clearDwell() {
    if (dwellTimer) clearTimeout(dwellTimer)
    dwellTimer = undefined
  }

  function resetMerge() {
    clearDwell()
    hoverTarget = null
    mergeArmedFor = null
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

    // 合并判定
    const id = tileAtPoint(ev.clientX, ev.clientY)
    const isSelf = !id || id === draggedId || id === SHADOW_PLACEHOLDER_ITEM_ID
    if (!isSelf) {
      if (id !== hoverTarget) {
        hoverTarget = id
        mergeArmedFor = null
        clearDwell()
        const candidate = id
        dwellTimer = setTimeout(() => {
          if (hoverTarget === candidate && draggedId) mergeArmedFor = candidate
        }, board.mergeDwellMs)
      }
    } else {
      resetMerge()
    }

    // 边缘翻页判定（拖拽中才生效）
    const dir: -1 | 1 | null =
      ev.clientX <= EDGE_PX ? -1 : ev.clientX >= window.innerWidth - EDGE_PX ? 1 : null
    if (dir !== edgeDir) {
      if (edgeTimer) clearTimeout(edgeTimer)
      edgeTimer = undefined
      edgeDir = dir
      const itemId = draggedId
      if (dir) {
        edgeTimer = setTimeout(() => {
          edgeTimer = undefined
          if (edgeDir === dir && draggedId && draggedId === itemId) onedge(dir, itemId)
        }, board.pageFlipEdgeMs)
      }
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
      syncMetrics()
    }
    const syncMetrics = () => {
      const rect = el.getBoundingClientRect()
      board.gridMetrics = { left: rect.left, top: rect.top, tile: tilePx, gap: gapPx, cols: displayCols }
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    window.addEventListener('scroll', syncMetrics, { passive: true })
    window.addEventListener('resize', syncMetrics)
    return () => {
      ro.disconnect()
      window.removeEventListener('scroll', syncMetrics)
      window.removeEventListener('resize', syncMetrics)
      detachTracking()
    }
  })

  function consider(e: CustomEvent<DndEvent<Item>>) {
    zoneItems = e.detail.items
    const trigger = e.detail.info.trigger
    if (trigger === TRIGGERS.DRAG_STARTED) {
      draggedId = e.detail.info.id
      resetMerge()
      clearEdge()
      captureHitCells()
      attachTracking()
    }
  }

  function finalize(e: CustomEvent<DndEvent<Item>>) {
    zoneItems = e.detail.items
    detachTracking()
    clearDwell()
    clearEdge()

    // 跨页 carry 已接管（beginCarry 会合成一次 mouseup 让库收场）：
    // 这一次 finalize 不能改数据，否则图标会在源页被重排。
    if (board.carry) {
      zoneItems = [...board.sequence]
      draggedId = null
      hoverTarget = null
      mergeArmedFor = null
      hitCells = []
      return
    }

    const source = draggedId
    const target = mergeArmedFor
    draggedId = null
    hoverTarget = null
    mergeArmedFor = null
    hitCells = []

    if (source && target && source !== target) {
      void board.mergeItems(source, target)
      return
    }

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

  // ---------- 长按 → 上下文菜单 ----------
  function onItemPointerDown(e: PointerEvent, item: Item) {
    if (e.button !== 0) return
    clearPress()
    pressStart = { x: e.clientX, y: e.clientY }
    window.addEventListener('pointermove', onPressMove, { passive: true })
    const target = item
    pressTimer = setTimeout(() => {
      pressTimer = undefined
      // 触屏长按：拖拽库会先在 300ms 起拖，这里先让它体面收场再弹菜单
      if (draggedId) {
        window.dispatchEvent(new MouseEvent('mouseup', { bubbles: true }))
        window.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, pointerType: 'mouse' }))
      }
      onmenu({ item: target, x: pressStart?.x ?? e.clientX, y: pressStart?.y ?? e.clientY })
      pressStart = null
    }, LONG_PRESS_MS)
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
          data-tile
          class="group relative"
          style="grid-column: {cell.col + 1} / span {cell.span}; grid-row: {cell.row + 1} / span {cell.span};"
          onpointerdown={(e) => onItemPointerDown(e, item)}
          onpointerup={clearPress}
          onpointercancel={clearPress}
          oncontextmenu={(e) => {
            e.preventDefault()
            clearPress()
            onmenu({ item, x: e.clientX, y: e.clientY })
          }}
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
