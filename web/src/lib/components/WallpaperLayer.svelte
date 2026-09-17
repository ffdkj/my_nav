<script lang="ts">
  import { board } from '$lib/store/board.svelte'

  import { wallpaperFull } from '$lib/wallpaper'
  // 轮换：off | load（每次进入随机） | interval（每 N 分钟）
  let rotatedId = $state<string | null>(null)
  let failed = $state(false)

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
</script>

<!--
  壁纸层：固定铺满视口，图片加载失败或没有壁纸时回落到可配的纯色。
  上面压一层暗色渐变，保证图标与文字在任何壁纸上都可读。
-->
<div class="pointer-events-none fixed inset-0 -z-20" aria-hidden="true">
  {#if src && !failed}
    <img
      src={src}
      alt=""
      class="size-full object-cover"
      onerror={() => (failed = true)}
      data-testid="wallpaper"
    />
  {:else}
    <div
      class="size-full"
      style="background: {board.settings['wallpaper_fallback'] || '#0b1220'}"
      data-testid="wallpaper-fallback"
    ></div>
  {/if}
  <div
    class="absolute inset-0 bg-gradient-to-b from-black/45 via-black/30 to-black/60"
    data-testid="wallpaper-scrim"
  ></div>
</div>
