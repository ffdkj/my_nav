<script lang="ts">
  import { board } from '$lib/store/board.svelte'
  import type { Item } from '$lib/types'

  interface Props {
    item: Item | null
    onclose: () => void
  }
  let { item, onclose }: Props = $props()

  const folder = $derived(item ? board.folderOf(item) : undefined)
  const count = $derived(item ? item.children.length : 0)

  let name = $state('')
  let size = $state<1 | 2>(1)

  $effect(() => {
    if (item) {
      name = folder?.name ?? ''
      size = (item.size ?? 1) as 1 | 2
    }
  })

  async function save() {
    if (!item) return
    if ((folder?.name ?? '') !== name) await board.renameFolder(item.id, name)
    if ((item.size ?? 1) !== size) await board.setFolderSize(item.id, size)
    onclose()
  }
</script>

{#if item}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) onclose()
    }}
  >
    <div class="w-full max-w-md rounded-2xl bg-surface-800 p-6 ring-1 ring-white/15" role="dialog" aria-modal="true" aria-label="编辑文件夹">
      <h2 class="mb-4 text-base font-semibold">编辑文件夹</h2>

      <label class="mb-4 block text-sm">
        <span class="mb-1 block text-white/70">名称（留空则显示图标数量）</span>
        <input
          bind:value={name}
          class="w-full rounded-lg bg-white/10 px-3 py-2 text-sm ring-1 ring-white/15 outline-none focus:ring-2 focus:ring-accent-500"
          placeholder="例如 开发工具"
          autocomplete="off"
        />
      </label>

      <fieldset class="mb-4 text-sm">
        <legend class="mb-1 text-white/70">形态</legend>
        <div class="flex gap-2">
          <button
            type="button"
            onclick={() => (size = 1)}
            class="flex-1 cursor-pointer rounded-lg px-3 py-2 ring-1 {size === 1
              ? 'bg-accent-500 text-white ring-accent-500'
              : 'bg-white/5 text-white/80 ring-white/15 hover:bg-white/10'}"
          >
            1×1（点击开模态）
          </button>
          <button
            type="button"
            onclick={() => (size = 2)}
            class="flex-1 cursor-pointer rounded-lg px-3 py-2 ring-1 {size === 2
              ? 'bg-accent-500 text-white ring-accent-500'
              : 'bg-white/5 text-white/80 ring-white/15 hover:bg-white/10'}"
          >
            2×2（内部直接可点）
          </button>
        </div>
        <p class="mt-1 text-xs text-white/40">当前 {count} 个图标；2×2 会占用 4 个格子。</p>
      </fieldset>

      <div class="flex justify-between gap-2">
        <button
          type="button"
          onclick={async () => {
            if (item && window.confirm(`删除文件夹及其 ${count} 个图标？`)) {
              await board.removeItem(item.id)
              onclose()
            }
          }}
          class="cursor-pointer rounded-lg bg-red-500/80 px-4 py-2 text-sm hover:bg-red-500"
        >
          删除
        </button>
        <div class="flex gap-2">
          <button
            type="button"
            onclick={onclose}
            class="cursor-pointer rounded-lg bg-white/10 px-4 py-2 text-sm hover:bg-white/20"
          >
            取消
          </button>
          <button
            type="button"
            onclick={save}
            class="cursor-pointer rounded-lg bg-accent-500 px-4 py-2 text-sm font-medium text-white hover:brightness-110"
          >
            确定
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
