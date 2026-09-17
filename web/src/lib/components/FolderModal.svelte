<script lang="ts">
  import { dndzone, TRIGGERS, type DndEvent } from 'svelte-dnd-action'
  import MiniIcon from '$lib/components/MiniIcon.svelte'
  import { board, MAX_FOLDER_ITEMS } from '$lib/store/board.svelte'
  import type { Child, Item } from '$lib/types'

  interface Props {
    item: Item | null
    onclose: () => void
  }
  let { item, onclose }: Props = $props()

  const folder = $derived(item ? board.folderOf(item) : undefined)
  const title = $derived(folder?.name || '文件夹')

  // 嵌套 zone：type 必须与外层网格不同，否则父子两个 zone 会互相抢
  let zoneItems = $state<Child[]>([])
  $effect(() => {
    zoneItems = item ? [...item.children].sort((a, b) => a.sort_order - b.sort_order) : []
  })

  let draggedChildId: string | null = null

  function consider(e: CustomEvent<DndEvent<Child>>) {
    zoneItems = e.detail.items
    if (e.detail.info.trigger === TRIGGERS.DRAG_STARTED) draggedChildId = e.detail.info.id
  }

  function finalize(e: CustomEvent<DndEvent<Child>>) {
    zoneItems = e.detail.items
    const trigger = e.detail.info.trigger
    const dragged = draggedChildId
    draggedChildId = null
    if (!item) return

    // 拖出面板 = 回到主网格（spec §10 / Q15 决策）
    if (trigger === TRIGGERS.DROPPED_OUTSIDE_OF_ANY && dragged) {
      void board.removeFromFolder(item.id, dragged)
      return
    }
    const ids = zoneItems.map((c) => c.id)
    const current = [...item.children].sort((a, b) => a.sort_order - b.sort_order).map((c) => c.id)
    if (ids.join('\u0000') !== current.join('\u0000')) {
      void board.reorderFolderChildren(item.id, ids)
    }
  }

  function eject(child: Child) {
    if (item) void board.removeFromFolder(item.id, child.id)
  }
</script>

{#if item}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4 backdrop-blur-md"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) onclose()
    }}
  >
    <div
      class="w-full max-w-sm rounded-2xl bg-surface-800/85 p-4 ring-1 ring-fg/15"
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div class="mb-3 flex items-center justify-between">
        <h2 class="text-sm font-semibold">
          {title}
          <span class="ml-1 text-fg/40">{zoneItems.length}/{MAX_FOLDER_ITEMS}</span>
        </h2>
        <button
          type="button"
          onclick={onclose}
          class="cursor-pointer rounded-lg px-2 py-1 text-sm text-fg/60 hover:bg-fg/10 hover:text-fg"
          aria-label="关闭"
        >
          &#10005;
        </button>
      </div>

      <ul
        use:dndzone={{
          items: zoneItems,
          type: 'folder-item',
          flipDurationMs: 180,
          delayTouchStart: 300,
          useCursorForDetection: true,
        }}
        onconsider={consider}
        onfinalize={finalize}
        class="m-0 grid list-none grid-cols-3 gap-2 p-0 text-xs"
        aria-label="夹内图标"
      >
        {#each zoneItems as child (child.id)}
          <li class="group/mini relative">
            {#if board.links[child.link_id]}
              <span class="block [&>span]:size-auto">
                <MiniIcon link={board.links[child.link_id]} mode="live" />
              </span>
            {/if}
            <button
              type="button"
              onclick={() => eject(child)}
              class="absolute -top-1 -right-1 flex size-4 cursor-pointer items-center justify-center rounded-full bg-surface-700 text-[9px] text-fg/80 ring-1 ring-fg/20 opacity-0 transition group-hover/mini:opacity-100 hover:bg-red-500 hover:text-white"
              aria-label="移出到主网格"
              title="移出到主网格"
            >
              &#8599;
            </button>
          </li>
        {/each}
      </ul>

      <p class="mt-3 text-[11px] text-fg/40">
        拖动可排序；把图标拖出面板即回到主网格；上限 {MAX_FOLDER_ITEMS} 个。
      </p>
    </div>
  </div>
{/if}
