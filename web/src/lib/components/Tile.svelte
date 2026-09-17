<script lang="ts">
  import BigFolder from '$lib/components/BigFolder.svelte'
  import FolderTile from '$lib/components/FolderTile.svelte'
  import { board, initialOf } from '$lib/store/board.svelte'
  import type { Item } from '$lib/types'

  interface Props {
    item: Item
    /** 悬停 dwell 已完成：松手即合并到这一块 */
    mergeArmed?: boolean
    onopen?: () => void
    onedit: () => void
    ondelete: () => void
  }
  let { item, mergeArmed = false, onopen, onedit, ondelete }: Props = $props()

  const link = $derived(board.linkOf(item))
  const host = $derived.by(() => {
    if (!link) return ''
    try {
      return new URL(link.url).hostname.replace(/^www\./, '')
    } catch {
      return link.url
    }
  })

  const highlight = $derived(mergeArmed ? 'scale-105 ring-3 ring-accent-500' : '')
</script>

<div class="size-full transition {highlight}">
  {#if item.kind === 'folder'}
    {#if (item.size ?? 1) === 2}
      <BigFolder {item} {onedit} {ondelete} />
    {:else}
      <FolderTile {item} onopen={onopen ?? onedit} {onedit} {ondelete} />
    {/if}
  {:else if link}
    <a
      href={link.url}
      target={link.open_new_tab ? '_blank' : undefined}
      rel="noreferrer noopener"
      draggable="false"
      style="-webkit-user-drag:none"
      class="flex size-[var(--tile)] flex-col items-center justify-center gap-1 overflow-hidden
             rounded-[var(--radius-tile)] bg-fg/10 ring-1 ring-fg/15 backdrop-blur-sm transition
             hover:bg-fg/20 hover:ring-fg/30 focus-visible:ring-3 focus-visible:ring-accent-500 focus-visible:outline-none"
      aria-label={link.title || host}
      title={link.url}
    >
      {#if link.icon_status === 'ok' && link.icon_path}
        <img src="/icons/{link.icon_path}" alt="" class="size-8 object-contain" loading="lazy" />
      {:else}
        <!-- 兜底：纯色文字图标（M5 抓到真图标后由服务端替换） -->
        <span
          class="flex size-8 items-center justify-center rounded-full text-sm font-semibold text-white"
          style="background:{link.mono_color}"
        >
          {initialOf(link)}
        </span>
      {/if}
      <span class="max-w-[90%] truncate text-[11px] leading-none text-fg/85">
        {link.title || host}
      </span>
    </a>

    <div
      class="absolute -top-1 -right-1 flex gap-1 opacity-0 transition group-hover:opacity-100 focus-within:opacity-100"
    >
      <button
        type="button"
        onclick={onedit}
        class="flex size-5 cursor-pointer items-center justify-center rounded-full bg-surface-700 text-[10px] text-fg/80 ring-1 ring-fg/20 hover:bg-accent-500 hover:text-white"
        aria-label="编辑 {link.title}"
      >
        &#9998;
      </button>
      <button
        type="button"
        onclick={ondelete}
        class="flex size-5 cursor-pointer items-center justify-center rounded-full bg-surface-700 text-[10px] text-fg/80 ring-1 ring-fg/20 hover:bg-red-500 hover:text-white"
        aria-label="删除 {link.title}"
      >
        &#10005;
      </button>
    </div>
  {/if}
</div>
