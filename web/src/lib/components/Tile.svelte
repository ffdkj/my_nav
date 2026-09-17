<script lang="ts">
  import type { Link } from '$lib/types'

  interface Props {
    link: Link
    /** 合并 dwell 计时中：目标高亮 + 轻抖反馈 */
    mergeReady?: boolean
    onactivate?: (link: Link) => void
  }

  let { link, mergeReady = false, onactivate }: Props = $props()

  const host = $derived.by(() => {
    try {
      return new URL(link.url).hostname.replace(/^www\./, '')
    } catch {
      return link.url
    }
  })

  const initial = $derived((link.mono_text || host).slice(0, 1).toUpperCase())
</script>

<li
  data-tile
  class="group relative flex w-[var(--tile)] flex-col items-center gap-2"
  aria-label={link.title || host}
>
  <a
    href={link.url}
    target={link.open_new_tab ? '_blank' : undefined}
    rel="noreferrer noopener"
    onclick={(e) => {
      if (onactivate) {
        e.preventDefault()
        onactivate(link)
      }
    }}
    class="flex size-[var(--tile)] items-center justify-center overflow-hidden rounded-[var(--radius-tile)]
           bg-white/10 ring-1 ring-white/15 backdrop-blur transition
           hover:bg-white/20 focus-visible:ring-3 focus-visible:ring-accent-500 focus-visible:outline-none
           {mergeReady ? 'scale-110 ring-3 ring-accent-500' : ''}"
  >
    {#if link.icon_status === 'ok' && link.icon_path}
      <img src="/icons/{link.icon_path}" alt="" class="size-8 object-contain" loading="lazy" />
    {:else}
      <!-- 兜底：纯色文字图标（spec §7 第 4 步） -->
      <span
        class="flex size-full items-center justify-center text-2xl font-semibold"
        style="background:{link.mono_color}; font-size:{link.mono_font_size}px"
      >
        {initial}
      </span>
    {/if}
  </a>
  <span class="max-w-full truncate text-xs text-white/85">{link.title || host}</span>
</li>
