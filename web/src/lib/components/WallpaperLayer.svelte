<script lang="ts">
  import { board } from '$lib/store/board.svelte'

  import { fallbackColor, wallpaperFull } from '$lib/wallpaper'
  import type { Wallpaper } from '$lib/types'

  // 轮换：off | load（每次进入随机） | interval（每 N 分钟）
  let rotatedId = $state<string | null>(null)
  let failed = $state(false)
  /** 出场那一张单独记失败标记，否则它加载失败会把入场那张一起打成兜底色 */
  let outFailed = $state(false)

  const active = $derived(board.activeWallpaper)
  const rotation = $derived(board.settings['wallpaper_rotation'] ?? 'off')
  const intervalMin = $derived(Number(board.settings['wallpaper_interval_min'] ?? '30') || 30)

  // 随机挑一张：首次加载时决定一次，之后按 interval 换
  function pickRandom(): string | null {
    const list = board.wallpapers
    if (list.length === 0) return null
    return list[Math.floor(Math.random() * list.length)].id
  }

  $effect(() => {
    if (board.wallpapers.length === 0) {
      rotatedId = null
      return
    }
    if (rotation === 'off') {
      rotatedId = null
      return
    }
    if (!rotatedId) rotatedId = pickRandom()
    if (rotation !== 'interval') return
    const timer = setInterval(() => {
      rotatedId = pickRandom()
      failed = false
    }, intervalMin * 60_000)
    return () => clearInterval(timer)
  })

  const shown = $derived.by(() => {
    if (rotation !== 'off' && rotatedId) {
      return board.wallpapers.find((w) => w.id === rotatedId) ?? active
    }
    return active
  })

  const src = $derived(shown ? wallpaperFull(shown) : '')
  $effect(() => {
    // 换壁纸时重置失败标记
    if (src) failed = false
  })

  // ---------- 换页时的平移（只在"新旧页壁纸不同"时发生）----------

  const t = $derived(board.transition)

  /** 出场那一张具体是哪个壁纸对象（换页前那页生效的那张） */
  const outgoing = $derived.by(() => {
    const id = t?.fromWallpaper
    return id ? board.wallpapers.find((w) => w.id === id) : undefined
  })

  /**
   * 新旧页壁纸相同就不平移：同一张图左右两份拼在一起看不出位移，
   * 反而会在滑动过程中暴露接缝。所以只有真的换了壁纸才铺两层轨道。
   */
  const slides = $derived(Boolean(t) && (t?.fromWallpaper ?? null) !== (active?.id ?? null))

  const trackTransform = $derived(board.animating ? 'translateX(-50%)' : 'translateX(0%)')
  const trackTransition = $derived(
    t ? 'transform var(--page-slide-ms) var(--page-slide-ease)' : 'none',
  )

  /**
   * 蒙版只在"画面上真有照片"时铺：照片亮度不可预知，需要压一层保证图标文字可读；
   * 而没有壁纸时的兜底底色本来就是按主题挑的纯色，再压一层暗渐变就只是**平白发灰**
   * （用户的原话是"整个页面被蒙上了暗色的蒙版"）。换页过程中任一侧有图也要铺，
   * 否则轨道两侧会出现"一半有蒙版一半没有"的接缝。
   */
  const showScrim = $derived(
    Boolean((shown && !failed) || (slides && outgoing && !outFailed)),
  )

  $effect(() => {
    if (t) outFailed = false
  })

  function markFailed(isOutgoing: boolean) {
    if (isOutgoing) outFailed = true
    else failed = true
  }
</script>

{#snippet slot(w: Wallpaper | undefined, isOutgoing: boolean)}
  {#if w && !(isOutgoing ? outFailed : failed)}
    <img
      src={wallpaperFull(w)}
      alt=""
      class="size-full object-cover"
      onerror={() => markFailed(isOutgoing)}
      data-testid={isOutgoing ? 'wallpaper-out' : 'wallpaper'}
    />
  {:else}
    <div
      class="size-full"
      style="background: {fallbackColor(board.settings['wallpaper_fallback'])}"
      data-testid={isOutgoing ? 'wallpaper-fallback-out' : 'wallpaper-fallback'}
    ></div>
  {/if}
{/snippet}

<!--
  壁纸层：固定铺满视口，图片加载失败或没有壁纸时回落到兜底底色。
  有照片时压一层渐变蒙版保证图标与文字可读
  —— 蒙版**跟随主题**：深色压黑、浅色压白（`--wallpaper-scrim`，见 app.css）；
  没有照片时**不压**（兜底底色本身就该是干净的，再压一层就是"整页发暗"的元凶，
  也解释了为什么曾经连白天主题都像蒙了层黑）。

  换页时若新旧壁纸不同，这里铺两层并按方向平移；相同时保持单层不动。
-->
<div class="pointer-events-none fixed inset-0 -z-20 overflow-hidden" aria-hidden="true">
  {#if slides && t}
    <div
      class="flex h-full w-[200%]"
      data-testid="wallpaper-track"
      style="transform: {trackTransform}; transition: {trackTransition};"
    >
      {#if t.dir === 1}
        <div class="h-full w-1/2 shrink-0">{@render slot(outgoing, true)}</div>
        <div class="h-full w-1/2 shrink-0">{@render slot(shown, false)}</div>
      {:else}
        <div class="h-full w-1/2 shrink-0">{@render slot(shown, false)}</div>
        <div class="h-full w-1/2 shrink-0">{@render slot(outgoing, true)}</div>
      {/if}
    </div>
  {:else}
    <div class="size-full">{@render slot(shown, false)}</div>
  {/if}
  {#if showScrim}
    <div
      class="absolute inset-0"
      style="background-image: var(--wallpaper-scrim)"
      data-testid="wallpaper-scrim"
    ></div>
  {/if}
</div>
