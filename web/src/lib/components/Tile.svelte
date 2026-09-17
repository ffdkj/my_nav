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

  /**
   * 图标撑满图块（Q4）。
   * 唯一例外是**小尺寸位图**：16/32px 的 favicon 硬拉到 96px 只会变成一团糊，
   * 所以按原始边长缩到六成。SVG / ICO 的 icon_w 是 0（尺寸未知，但矢量或含多尺寸），
   * 一律按"能撑满"处理。
   */
  const iconFill = $derived(link?.icon_w && link.icon_w < 64 ? '60%' : '100%')
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
      class="@container relative flex size-[var(--tile)] items-center justify-center overflow-hidden
             rounded-[var(--radius-tile)] bg-glass ring-1 ring-glass-ring frosted transition
             hover:bg-glass-hover hover:ring-fg/30 focus-visible:ring-3 focus-visible:ring-accent-500 focus-visible:outline-none"
      aria-label={link.title || host}
      title={link.url}
      data-testid="tile-link"
    >
      {#if link.icon_status === 'ok' && link.icon_path}
        <img
          src="/icons/{link.icon_path}"
          alt=""
          class="object-contain"
          style="width:{iconFill};height:{iconFill}"
          loading="lazy"
          data-testid="tile-icon"
        />
      {:else}
        <!-- 兜底：纯色文字图标（服务端抓到真图标后由 replace 掉）—— 铺满整块，字母居中 -->
        <span
          class="flex size-full items-center justify-center font-semibold text-white"
          style="background:{link.mono_color}"
          data-testid="tile-monogram"
        >
          <span class="text-[42cqw] leading-none">{initialOf(link)}</span>
        </span>
      {/if}

      <!--
        文字不再常驻：图标要撑满容器。悬停/键盘聚焦时从底部浮出一条渐变压条，
        深底白字在两种主题下都成立（图块里可能是任意颜色的 logo）。
      -->
      <span
        class="pointer-events-none absolute inset-x-0 bottom-0 truncate bg-gradient-to-t from-black/80 to-transparent
               px-1.5 pt-4 pb-1 text-center text-[11px] leading-tight text-white opacity-0 transition-opacity duration-150
               group-hover:opacity-100 group-focus-within:opacity-100"
        data-testid="tile-label"
      >
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
