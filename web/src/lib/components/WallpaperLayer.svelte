<script lang="ts">
  import { board } from '$lib/store/board.svelte'
  import { theme } from '$lib/store/theme.svelte'

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

  /**
   * 轨道方向：**两个方向必须一个向左一个向右**。
   *
   * 槽位在下面按方向排（dir=+1 是 [旧, 新]，dir=-1 是 [新, 旧]），
   * 所以窗口的起点/终点也要跟着翻：
   *   dir=+1（下一页，内容从右侧进）→ 0% → -50%，壁纸整体向左
   *   dir=-1（上一页，内容从左侧进）→ -50% → 0%，壁纸整体向右
   * 之前两个方向都写死 0% → -50%，于是"上一页"会先闪出新壁纸、
   * 滑向旧壁纸、收尾再跳回新的 —— 三个错误叠在一起。
   */
  const trackTransform = $derived(
    board.animating
      ? `translate3d(${t?.dir === -1 ? 0 : -50}%, 0, 0)`
      : `translate3d(${t?.dir === -1 ? -50 : 0}%, 0, 0)`,
  )
  const trackTransition = $derived(
    t ? 'transform var(--page-slide-ms) var(--page-slide-ease)' : 'none',
  )

  /**
   * 画面上真的有照片（换页过程中任一侧有照片也算，否则轨道两侧会一半有一半没有）。
   * 玻璃面（图块/搜索栏/圆点）的配色与"要不要压蒙版"都看它。
   */
  const photoBehind = $derived(
    Boolean((shown && !failed) || (slides && outgoing && !outFailed)),
  )

  /**
   * 蒙版只在**深色主题**下铺：浅色主题下那层白蒙版会把壁纸整张洗成灰白（用户的原话是
   * "太怪了"），浅色的可读性改由白色玻璃面提供（见 app.css 的 `:root[data-photo]`）。
   * 没有照片时谁都不铺 —— 兜底底色本来就是按主题挑的纯色，再压一层就是平白发灰。
   */
  const showScrim = $derived(photoBehind && theme.dark)

  // 玻璃 token 的选择依据，写在 <html> 上（App 的主题标记也在那儿）。
  // 图片还没下下来时看到的是兜底底色，此时用"无照片"那一档更合适，所以这个标记
  // 早设一帧也没有坏处。
  $effect(() => {
    document.documentElement.dataset.photo = photoBehind ? '1' : ''
  })

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

  蒙版只在**深色主题 + 真有照片**时铺（`--wallpaper-scrim`，见 app.css）；
  浅色主题不铺蒙版（白蒙版会把壁纸洗掉），可读性靠 `--c-glass` 那组白色玻璃，
  由 `<html data-photo>` 切换。没有照片时谁都不铺，免得兜底底色平白发灰。

  换页时若新旧壁纸不同，这里铺两层并按方向平移（向左/向右由 `trackTransform` 决定）；
  相同时保持单层不动。
-->
<div class="pointer-events-none fixed inset-0 -z-20 overflow-hidden" aria-hidden="true">
  {#if slides && t}
    <div
      class="flex h-full w-[200%]"
      data-testid="wallpaper-track"
      style="transform: {trackTransform}; transition: {trackTransition}; will-change: transform;"
    >
      {#if t.dir === 1}
        <!-- 下一页：窗口起点在左半（旧壁纸），向右平移的窗口滑到右半（新壁纸） -->
        <div class="h-full w-1/2 shrink-0">{@render slot(outgoing, true)}</div>
        <div class="h-full w-1/2 shrink-0">{@render slot(shown, false)}</div>
      {:else}
        <!-- 上一页：新壁纸在左、旧壁纸在右，窗口从右半滑回左半（整体向右） -->
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
